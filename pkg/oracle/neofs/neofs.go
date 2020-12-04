package neofs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/mr-tron/base58"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neofs-api-go/pkg/client"
	"github.com/nspcc-dev/neofs-api-go/pkg/container"
	"github.com/nspcc-dev/neofs-api-go/pkg/object"
	objectv2 "github.com/nspcc-dev/neofs-api-go/v2/object"
)

const (
	// URIScheme is the name of neofs URI scheme.
	URIScheme = "neofs"

	// containerIDSize is the size of container id in bytes.
	containerIDSize = sha256.Size
	// objectIDSize is the size of container id in bytes.
	objectIDSize = sha256.Size
	// rangeSep is a separator between offset and length.
	rangeSep = '|'

	rangeCmd  = "range"
	headerCmd = "header"
	hashCmd   = "hash"
)

// Various validation errors.
var (
	ErrInvalidScheme    = errors.New("invalid URI scheme")
	ErrMissingObject    = errors.New("object ID is missing from URI")
	ErrInvalidContainer = errors.New("container ID is invalid")
	ErrInvalidObject    = errors.New("object ID is invalid")
	ErrInvalidRange     = errors.New("object range is invalid (expected 'Offset|Length'")
	ErrInvalidCommand   = errors.New("invalid command")
)

// Get returns neofs object from the provided url.
// URI scheme is "neofs://<Container-ID>/<Object-ID/<Command>/<Params>".
// If Command is not provided, full object is requested.
// TODO multiple addresses
// 	Addresses are queried sequentially using the same context.
func Get(ctx context.Context, priv *keys.PrivateKey, u *url.URL, addr string) ([]byte, error) {
	if u.Scheme != URIScheme {
		return nil, ErrInvalidScheme
	}
	ps := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(ps) == 0 {
		return nil, ErrMissingObject
	}

	rawCID, err := base58.Decode(u.Hostname())
	if err != nil || len(rawCID) != containerIDSize {
		return nil, ErrInvalidContainer
	}
	rawOID, err := base58.Decode(ps[0])
	if err != nil || len(rawOID) != objectIDSize {
		return nil, ErrInvalidObject
	}

	var cid, oid [32]byte
	copy(cid[:], rawCID)
	containerID := container.NewID()
	containerID.SetSHA256(cid)

	copy(oid[:], rawOID)
	objectID := object.NewID()
	objectID.SetSHA256(oid)

	objectAddr := object.NewAddress()
	objectAddr.SetContainerID(containerID)
	objectAddr.SetObjectID(objectID)

	c, err := client.New(&priv.PrivateKey, client.WithAddress(addr))
	if err != nil {
		return nil, err
	}

	switch {
	case len(ps) == 1: // Get request
		return getPayload(ctx, c, objectAddr)
	case ps[1] == rangeCmd:
		return getRange(ctx, c, objectAddr, ps[2:]...)
	case ps[1] == headerCmd:
		return getHeader(ctx, c, objectAddr)
	case ps[1] == hashCmd:
		return getHash(ctx, c, objectAddr, ps[2:]...)
	default:
		return nil, ErrInvalidCommand
	}
}

func getPayload(ctx context.Context, c *client.Client, addr *object.Address) ([]byte, error) {
	obj, err := c.GetObject(ctx, new(client.GetObjectParams).WithAddress(addr))
	if err != nil {
		return nil, err
	}
	return obj.GetPayload(), nil
}

func getRange(ctx context.Context, c *client.Client, addr *object.Address, ps ...string) ([]byte, error) {
	if len(ps) == 0 {
		return nil, ErrInvalidRange
	}
	r, err := parseRange(ps[0])
	if err != nil {
		return nil, err
	}
	return c.ObjectPayloadRangeData(ctx, new(client.RangeDataParams).WithAddress(addr).WithRange(r))
}

func getHeader(ctx context.Context, c *client.Client, addr *object.Address) ([]byte, error) {
	obj, err := c.GetObjectHeader(ctx, new(client.ObjectHeaderParams).WithAddress(addr))
	if err != nil {
		return nil, err
	}
	msg := objectv2.ObjectToGRPCMessage(obj.ToV2()).Header
	b := bytes.NewBuffer(nil)
	err = new(jsonpb.Marshaler).Marshal(b, msg)
	return b.Bytes(), err
}

func getHash(ctx context.Context, c *client.Client, addr *object.Address, ps ...string) ([]byte, error) {
	if len(ps) == 0 || ps[0] == "" { // hash of the full payload
		obj, err := c.GetObjectHeader(ctx, new(client.ObjectHeaderParams).WithAddress(addr))
		if err != nil {
			return nil, err
		}
		return obj.GetPayloadChecksum().GetSum(), nil
	}
	r, err := parseRange(ps[0])
	if err != nil {
		return nil, err
	}
	hashes, err := c.ObjectPayloadRangeSHA256(ctx,
		new(client.RangeChecksumParams).WithAddress(addr).WithRangeList(r))
	if err != nil {
		return nil, err
	}
	if len(hashes) == 0 {
		return nil, fmt.Errorf("%w: empty response", ErrInvalidRange)
	}
	return hashes[0][:], nil
}

func parseRange(s string) (*object.Range, error) {
	sepIndex := strings.IndexByte(s, rangeSep)
	if sepIndex < 0 {
		return nil, ErrInvalidRange
	}
	offset, err := strconv.ParseUint(s[:sepIndex], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid offset", ErrInvalidRange)
	}
	length, err := strconv.ParseUint(s[sepIndex+1:], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid length", ErrInvalidRange)
	}

	r := object.NewRange()
	r.SetOffset(offset)
	r.SetLength(length)
	return r, nil
}
