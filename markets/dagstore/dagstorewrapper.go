package dagstore

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/dagstore"
	"github.com/filecoin-project/dagstore/mount"
	"github.com/filecoin-project/dagstore/shard"
	"github.com/filecoin-project/go-fil-markets/carstore"
	"github.com/filecoin-project/go-fil-markets/shared"
)

var log = logging.Logger("dagstore-wrapper")
var gcInterval = 5 * time.Minute

// MarketDAGStoreConfig is the config the market needs to then construct a DAG Store.
type MarketDAGStoreConfig struct {
	TransientsDir string
	IndexDir      string
	Datastore     ds.Datastore
}

type closableBlockstore struct {
	bstore.Blockstore
	io.Closer
}

type dagStoreWrapper struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	dagStore *dagstore.DAGStore
	mountApi LotusMountAPI
}

var _ shared.DagStoreWrapper = (*dagStoreWrapper)(nil)

func NewDagStoreWrapper(cfg MarketDAGStoreConfig, mountApi LotusMountAPI) (*dagStoreWrapper, error) {
	// construct the DAG Store.
	registry := mount.NewRegistry()
	if err := registry.Register(lotusScheme, NewLotusMountTemplate(mountApi)); err != nil {
		return nil, xerrors.Errorf("failed to create registry: %w", err)
	}

	failureCh := make(chan dagstore.ShardResult, 1)
	dcfg := dagstore.Config{
		TransientsDir: cfg.TransientsDir,
		IndexDir:      cfg.IndexDir,
		Datastore:     cfg.Datastore,
		MountRegistry: registry,
		FailureCh:     failureCh,
	}
	dagStore, err := dagstore.NewDAGStore(dcfg)
	if err != nil {
		return nil, xerrors.Errorf("failed to create DAG store: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	dw := &dagStoreWrapper{
		ctx:    ctx,
		cancel: cancel,

		dagStore: dagStore,
		mountApi: mountApi,
	}

	dw.wg.Add(1)
	// the dagstore will write Shard failures to the `failureCh` here. Run a go-routine to handle them.
	go dw.handleFailures(failureCh)

	return dw, nil
}

func (ds *dagStoreWrapper) handleFailures(failureCh chan dagstore.ShardResult) {
	defer ds.wg.Done()
	ticker := time.NewTicker(gcInterval)
	defer ticker.Stop()

	select {
	case <-ticker.C:
		_, _ = ds.dagStore.GC(ds.ctx)
	case f := <-failureCh:
		log.Errorw("shard failed", "shard-key", f.Key.String(), "error", f.Error)
		if err := ds.dagStore.RecoverShard(ds.ctx, f.Key, nil, dagstore.RecoverOpts{}); err != nil {
			log.Warnw("shard recovery failed", "shard-key", f.Key.String(), "error", err)
		}
	case <-ds.ctx.Done():
		return
	}
}

func (ds *dagStoreWrapper) LoadShard(ctx context.Context, pieceCid cid.Cid) (carstore.ClosableBlockstore, error) {
	key := shard.KeyFromCID(pieceCid)
	resch := make(chan dagstore.ShardResult, 1)
	err := ds.dagStore.AcquireShard(ctx, key, resch, dagstore.AcquireOpts{})

	if err != nil {
		if xerrors.Unwrap(err) != dagstore.ErrShardUnknown {
			return nil, xerrors.Errorf("failed to schedule acquire shard for piece CID %s: %w", pieceCid, err)
		}

		// if the DAGStore does not know about the Shard -> register it and then try to acquire it again.
		log.Warnw("failed to load shard as shard is not registered, will re-register", "pieceCID", pieceCid)
		if err := shared.RegisterShardSync(ctx, ds, pieceCid, "", false); err != nil {
			return nil, xerrors.Errorf("failed to re-register shard during loading piece CID %s: %w", pieceCid, err)
		}
		log.Warnw("successfully re-registered shard", "pieceCID", pieceCid)

		resch = make(chan dagstore.ShardResult, 1)
		if err := ds.dagStore.AcquireShard(ctx, key, resch, dagstore.AcquireOpts{}); err != nil {
			return nil, xerrors.Errorf("failed to acquire Shard for piece CID %s after re-registering: %w", pieceCid, err)
		}
	}

	// TODO: Can I rely on AcquireShard to return an error if the context times out?
	var res dagstore.ShardResult
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res = <-resch:
		if res.Error != nil {
			return nil, xerrors.Errorf("failed to acquire shard for piece CID %s: %w", pieceCid, res.Error)
		}
	}

	bs, err := res.Accessor.Blockstore()
	if err != nil {
		return nil, err
	}

	return &closableBlockstore{Blockstore: NewReadOnlyBlockstore(bs), Closer: res.Accessor}, nil
}

func (ds *dagStoreWrapper) RegisterShard(ctx context.Context, pieceCid cid.Cid, carPath string, eagerInit bool, resch chan dagstore.ShardResult) error {
	// Create a lotus mount with the piece CID
	key := shard.KeyFromCID(pieceCid)
	mt, err := NewLotusMount(pieceCid, ds.mountApi)
	if err != nil {
		return xerrors.Errorf("failed to create lotus mount for piece CID %s: %w", pieceCid, err)
	}

	// Register the shard
	opts := dagstore.RegisterOpts{
		ExistingTransient:  carPath,
		LazyInitialization: !eagerInit,
	}
	err = ds.dagStore.RegisterShard(ctx, key, mt, resch, opts)
	if err != nil {
		return xerrors.Errorf("failed to schedule register shard for piece CID %s: %w", pieceCid, err)
	}

	return nil
}

func (ds *dagStoreWrapper) Close() error {
	if err := ds.dagStore.Close(); err != nil {
		return err
	}

	ds.cancel()
	ds.wg.Wait()

	return nil
}
