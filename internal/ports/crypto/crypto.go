package cryptoport

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/d0lim/cockpit/internal/ports"
)

const maxRandomBytes = 65536

var hashes = map[string]crypto.Hash{"SHA-256": crypto.SHA256, "SHA-384": crypto.SHA384, "SHA-512": crypto.SHA512}

type KeyRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type KeyPair struct {
	PublicKey  KeyRef `json:"publicKey"`
	PrivateKey KeyRef `json:"privateKey"`
}

type entry struct {
	key     *rsa.PrivateKey
	hash    crypto.Hash
	private bool
}

type Keys struct {
	mu      sync.Mutex
	mask    ports.Mask
	entries map[string]entry
}

func NewKeys(mask ports.Mask) *Keys {
	if mask == nil {
		mask = ports.Identity
	}
	return &Keys{mask: mask, entries: map[string]entry{}}
}

func (k *Keys) Generate(ctx context.Context, bits int, hashName string) (KeyPair, error) {
	hash, ok := hashes[hashName]
	if !ok {
		return KeyPair{}, ports.BadArgs(k.mask, "unsupported hash "+hashName)
	}
	if bits < 2048 || bits > 4096 {
		return KeyPair{}, ports.BadArgs(k.mask, fmt.Sprintf("key length %d is outside 2048-4096 bits", bits))
	}
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return KeyPair{}, ports.Unavailable(k.mask, "failed to generate the key")
	}
	if err := ctx.Err(); err != nil {
		return KeyPair{}, ports.Unavailable(k.mask, "key generation was cancelled")
	}
	pair := KeyPair{PublicKey: KeyRef{Type: "public", ID: newID()}, PrivateKey: KeyRef{Type: "private", ID: newID()}}
	k.mu.Lock()
	k.entries[pair.PublicKey.ID] = entry{key: key, hash: hash}
	k.entries[pair.PrivateKey.ID] = entry{key: key, hash: hash, private: true}
	k.mu.Unlock()
	return pair, nil
}

func (k *Keys) lookup(id string) (entry, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	e, ok := k.entries[id]
	if !ok {
		return entry{}, ports.BadArgs(k.mask, "unknown key reference "+id)
	}
	return e, nil
}

func (k *Keys) Sign(id string, data []byte) ([]byte, error) {
	e, err := k.lookup(id)
	if err != nil {
		return nil, err
	}
	if !e.private {
		return nil, ports.Denied(k.mask, "cannot sign with a public key")
	}
	sig, err := SignPKCS1v15(e.key, e.hash, data)
	if err != nil {
		return nil, ports.Unavailable(k.mask, "failed to create the signature")
	}
	return sig, nil
}

func (k *Keys) PublicKey(id string) (*rsa.PublicKey, error) {
	e, err := k.lookup(id)
	if err != nil {
		return nil, err
	}
	return &e.key.PublicKey, nil
}

func SignPKCS1v15(key *rsa.PrivateKey, hash crypto.Hash, data []byte) ([]byte, error) {
	h := hash.New()
	h.Write(data)
	return rsa.SignPKCS1v15(rand.Reader, key, hash, h.Sum(nil))
}

func RandomUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func RandomBytes(mask ports.Mask, n int) ([]byte, error) {
	if n < 0 {
		return nil, ports.BadArgs(mask, "random byte length is negative")
	}
	if n > maxRandomBytes {
		return nil, ports.Limit(mask, fmt.Sprintf("random byte length %d exceeds the limit %d", n, maxRandomBytes))
	}
	out := make([]byte, n)
	if _, err := rand.Read(out); err != nil {
		return nil, ports.Unavailable(mask, "failed to generate random bytes")
	}
	return out, nil
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
