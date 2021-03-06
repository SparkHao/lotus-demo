// Code generated by github.com/whyrusleeping/cbor-gen. DO NOT EDIT.

package shedgen

import (
	"fmt"
	"io"
	"math"
	"sort"

	cid "github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	xerrors "golang.org/x/xerrors"
)

var _ = xerrors.Errorf
var _ = cid.Undef
var _ = math.E
var _ = sort.Sort

func (t *CarbNode) MarshalCBOR(w io.Writer) error {
	if t == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}
	if _, err := w.Write([]byte{161}); err != nil {
		return err
	}

	scratch := make([]byte, 9)

	// t.Sub ([]cid.Cid) (slice)
	if len("Sub") > cbg.MaxLength {
		return xerrors.Errorf("Value in field \"Sub\" was too long")
	}

	if err := cbg.WriteMajorTypeHeaderBuf(scratch, w, cbg.MajTextString, uint64(len("Sub"))); err != nil {
		return err
	}
	if _, err := io.WriteString(w, string("Sub")); err != nil {
		return err
	}

	if len(t.Sub) > cbg.MaxLength {
		return xerrors.Errorf("Slice value in field t.Sub was too long")
	}

	if err := cbg.WriteMajorTypeHeaderBuf(scratch, w, cbg.MajArray, uint64(len(t.Sub))); err != nil {
		return err
	}
	for _, v := range t.Sub {
		if err := cbg.WriteCidBuf(scratch, w, v); err != nil {
			return xerrors.Errorf("failed writing cid field t.Sub: %w", err)
		}
	}
	return nil
}

func (t *CarbNode) UnmarshalCBOR(r io.Reader) error {
	*t = CarbNode{}

	br := cbg.GetPeeker(r)
	scratch := make([]byte, 8)

	maj, extra, err := cbg.CborReadHeaderBuf(br, scratch)
	if err != nil {
		return err
	}
	if maj != cbg.MajMap {
		return fmt.Errorf("cbor input should be of type map")
	}

	if extra > cbg.MaxLength {
		return fmt.Errorf("CarbNode: map struct too large (%d)", extra)
	}

	var name string
	n := extra

	for i := uint64(0); i < n; i++ {

		{
			sval, err := cbg.ReadStringBuf(br, scratch)
			if err != nil {
				return err
			}

			name = string(sval)
		}

		switch name {
		// t.Sub ([]cid.Cid) (slice)
		case "Sub":

			maj, extra, err = cbg.CborReadHeaderBuf(br, scratch)
			if err != nil {
				return err
			}

			if extra > cbg.MaxLength {
				return fmt.Errorf("t.Sub: array too large (%d)", extra)
			}

			if maj != cbg.MajArray {
				return fmt.Errorf("expected cbor array")
			}

			if extra > 0 {
				t.Sub = make([]cid.Cid, extra)
			}

			for i := 0; i < int(extra); i++ {

				c, err := cbg.ReadCid(br)
				if err != nil {
					return xerrors.Errorf("reading cid field t.Sub failed: %w", err)
				}
				t.Sub[i] = c
			}

		default:
			// Field doesn't exist on this type, so ignore it
			cbg.ScanForLinks(r, func(cid.Cid) {})
		}
	}

	return nil
}
