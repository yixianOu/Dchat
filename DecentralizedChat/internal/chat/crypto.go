package chat

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/nacl/box"
)

// b64 encodes bytes to standard base64
func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// b64dec decodes standard base64
func b64dec(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }

// encryptDirect performs public-key encryption using NaCl box (X25519 + XSalsa20-Poly1305)
// Keys are expected raw 32 bytes each, base64 encoded.
func encryptDirect(senderPrivB64, recipientPubB64 string, plaintext []byte) (nonceB64, cipherB64 string, err error) {
	privRaw, err := b64dec(senderPrivB64)
	if err != nil {
		return "", "", fmt.Errorf("decode sender priv: %w", err)
	}
	peerPubRaw, err := b64dec(recipientPubB64)
	if err != nil {
		return "", "", fmt.Errorf("decode recipient pub: %w", err)
	}
	if len(privRaw) != 32 || len(peerPubRaw) != 32 {
		return "", "", errors.New("key size invalid (need 32 bytes raw)")
	}
	var priv [32]byte
	copy(priv[:], privRaw)
	var peerPub [32]byte
	copy(peerPub[:], peerPubRaw)
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", "", err
	}
	sealed := box.Seal(nil, plaintext, &nonce, &peerPub, &priv)
	return b64(nonce[:]), b64(sealed), nil
}

// decryptDirect decrypts a message encrypted with encryptDirect
func decryptDirect(recipientPrivB64, senderPubB64, nonceB64, cipherB64 string) ([]byte, error) {
	privRaw, err := b64dec(recipientPrivB64)
	if err != nil {
		return nil, fmt.Errorf("decode priv: %w", err)
	}
	pubRaw, err := b64dec(senderPubB64)
	if err != nil {
		return nil, fmt.Errorf("decode pub: %w", err)
	}
	nonceRaw, err := b64dec(nonceB64)
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}
	cipherRaw, err := b64dec(cipherB64)
	if err != nil {
		return nil, fmt.Errorf("decode cipher: %w", err)
	}
	if len(privRaw) != 32 || len(pubRaw) != 32 || len(nonceRaw) != 24 {
		return nil, errors.New("invalid sizes")
	}
	var priv [32]byte
	copy(priv[:], privRaw)
	var pub [32]byte
	copy(pub[:], pubRaw)
	var nonce [24]byte
	copy(nonce[:], nonceRaw)
	out, ok := box.Open(nil, cipherRaw, &nonce, &pub, &priv)
	if !ok {
		return nil, errors.New("decrypt failed")
	}
	return out, nil
}

// encryptGroup uses AES-256-GCM with a base64 encoded 32-byte key
func encryptGroup(symKeyB64 string, plaintext []byte) (nonceB64, cipherB64 string, err error) {
	key, err := b64dec(symKeyB64)
	if err != nil {
		return "", "", fmt.Errorf("decode group key: %w", err)
	}
	if len(key) != 32 {
		return "", "", errors.New("group key must be 32 bytes raw")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	return b64(nonce), b64(sealed), nil
}

// decryptGroup decrypts AES-256-GCM ciphertext
func decryptGroup(symKeyB64, nonceB64, cipherB64 string) ([]byte, error) {
	key, err := b64dec(symKeyB64)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("group key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, err := b64dec(nonceB64)
	if err != nil {
		return nil, err
	}
	cipherRaw, err := b64dec(cipherB64)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("nonce size mismatch")
	}
	pt, err := gcm.Open(nil, nonce, cipherRaw, nil)
	if err != nil {
		return nil, err
	}
	return pt, nil
}
