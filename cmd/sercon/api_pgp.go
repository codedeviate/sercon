package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

// PGP support for api.crypto.encrypt.*. The age path stays the default;
// these functions are reached only when detectBackend classifies a
// recipient / identity / ciphertext as PGP. The dispatch lives in
// api_encrypt.go (encryptEncrypt / encryptDecrypt); this file holds
// the openpgp mechanics so the age code stays uncluttered.
//
// Library: github.com/ProtonMail/go-crypto/openpgp — the maintained
// pure-Go PGP fork (x/crypto/openpgp is frozen / deprecated). We
// expose a deliberately small subset: generate a keypair, encrypt
// for public keys, decrypt with private keys. No signing, no
// subkey management, no web-of-trust — those are out of scope.

// pgpKeygen generates a fresh PGP keypair (RSA 2048 by default via
// openpgp's config) and returns both halves as ASCII-armored
// blocks. `name` / `email` populate the primary user ID; both are
// optional (empty strings are fine for throwaway keys).
func pgpKeygen(name, email string) (publicKey, privateKey string, err error) {
	entity, err := openpgp.NewEntity(name, "", email, nil)
	if err != nil {
		return "", "", fmt.Errorf("pgp keygen: %w", err)
	}

	pubArmored, err := armorEntity(entity, false)
	if err != nil {
		return "", "", err
	}
	privArmored, err := armorEntity(entity, true)
	if err != nil {
		return "", "", err
	}
	return pubArmored, privArmored, nil
}

// armorEntity serialises an entity to an ASCII-armored block —
// public (PGP PUBLIC KEY BLOCK) or private (PGP PRIVATE KEY BLOCK).
func armorEntity(entity *openpgp.Entity, private bool) (string, error) {
	var buf bytes.Buffer
	blockType := openpgp.PublicKeyType
	if private {
		blockType = openpgp.PrivateKeyType
	}
	w, err := armor.Encode(&buf, blockType, nil)
	if err != nil {
		return "", fmt.Errorf("pgp armor: %w", err)
	}
	if private {
		err = entity.SerializePrivate(w, nil)
	} else {
		err = entity.Serialize(w)
	}
	if err != nil {
		_ = w.Close()
		return "", fmt.Errorf("pgp serialize: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("pgp armor close: %w", err)
	}
	return buf.String(), nil
}

// pgpEncrypt seals `data` to one or more PGP public-key blocks. The
// output is always ASCII-armored (PGP MESSAGE) — PGP's binary form
// exists but armored is the portable default and matches what
// scripts paste around. Mirrors the age multi-recipient behaviour:
// any one of the recipients' private keys can decrypt.
func pgpEncrypt(data []byte, recipientBlocks []string) ([]byte, error) {
	recipients := make([]*openpgp.Entity, 0, len(recipientBlocks))
	for _, block := range recipientBlocks {
		el, err := openpgp.ReadArmoredKeyRing(strings.NewReader(block))
		if err != nil {
			return nil, fmt.Errorf("pgp: parse recipient public key: %w", err)
		}
		recipients = append(recipients, el...)
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("pgp: no usable recipient keys")
	}

	var buf bytes.Buffer
	armorW, err := armor.Encode(&buf, "PGP MESSAGE", nil)
	if err != nil {
		return nil, fmt.Errorf("pgp: armor: %w", err)
	}
	plainW, err := openpgp.Encrypt(armorW, recipients, nil, nil, nil)
	if err != nil {
		_ = armorW.Close()
		return nil, fmt.Errorf("pgp: encrypt: %w", err)
	}
	if _, err := plainW.Write(data); err != nil {
		_ = plainW.Close()
		_ = armorW.Close()
		return nil, fmt.Errorf("pgp: write: %w", err)
	}
	if err := plainW.Close(); err != nil {
		_ = armorW.Close()
		return nil, fmt.Errorf("pgp: encrypt close: %w", err)
	}
	if err := armorW.Close(); err != nil {
		return nil, fmt.Errorf("pgp: armor close: %w", err)
	}
	return buf.Bytes(), nil
}

// pgpDecrypt opens a PGP message with one of the supplied private-key
// blocks. Accepts both armored (`-----BEGIN PGP MESSAGE-----`) and
// binary ciphertext — armor is auto-detected by sniffing the leading
// bytes. Returns the plaintext bytes.
func pgpDecrypt(ciphertext []byte, identityBlocks []string) ([]byte, error) {
	var keyring openpgp.EntityList
	for _, block := range identityBlocks {
		el, err := openpgp.ReadArmoredKeyRing(strings.NewReader(block))
		if err != nil {
			return nil, fmt.Errorf("pgp: parse identity private key: %w", err)
		}
		keyring = append(keyring, el...)
	}
	if len(keyring) == 0 {
		return nil, fmt.Errorf("pgp: no usable identity keys")
	}

	// Armor-detect: a PGP MESSAGE block starts with `-----BEGIN`. The
	// binary form starts with a packet tag byte (high bit set). Decode
	// the armor layer first when present.
	var msgReader io.Reader = bytes.NewReader(ciphertext)
	if bytes.HasPrefix(bytes.TrimSpace(ciphertext), []byte("-----BEGIN PGP")) {
		block, err := armor.Decode(bytes.NewReader(ciphertext))
		if err != nil {
			return nil, fmt.Errorf("pgp: armor decode: %w", err)
		}
		msgReader = block.Body
	}

	md, err := openpgp.ReadMessage(msgReader, keyring, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("pgp: read message: %w", err)
	}
	plain, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		return nil, fmt.Errorf("pgp: read body: %w", err)
	}
	return plain, nil
}

// looksPGPMessage reports whether ciphertext is a PGP message — used
// by encryptDecrypt to route to the PGP path. Only the armored form
// is recognised by prefix; binary PGP messages aren't auto-detected
// (they share no stable magic with age's binary format that we'd
// want to disambiguate on), so callers relying on binary PGP must
// pass PGP private-key identities, which is the stronger signal the
// dispatch actually uses.
func looksPGPMessage(b []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(b), []byte("-----BEGIN PGP MESSAGE-----"))
}
