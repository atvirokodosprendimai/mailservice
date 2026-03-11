package edproof

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrChallengeExpired  = errors.New("challenge expired")
	ErrChallengeTampered = errors.New("challenge tampered or invalid")
	ErrChallengeFuture   = errors.New("challenge timestamp is in the future")
	ErrSignatureInvalid  = errors.New("signature verification failed")
)

const (
	challengeVersion  = "v1"
	challengeNonceLen = 16 // bytes
	maxClockSkew      = 5 * time.Second
)

// GenerateChallenge creates a stateless HMAC-authenticated challenge string.
// Format: v1.<unix_ts>.<hex_nonce>.<hmac_hex>
// The HMAC covers: v1.<ts>.<nonce>.<canonical_pubkey_blob>
func GenerateChallenge(pubkey string, secret []byte, now time.Time) (string, error) {
	blob, err := extractPubkeyBlob(pubkey)
	if err != nil {
		return "", fmt.Errorf("invalid public key: %w", err)
	}

	nonce := make([]byte, challengeNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ts := strconv.FormatInt(now.Unix(), 10)
	nonceHex := hex.EncodeToString(nonce)

	hmacInput := challengeVersion + "." + ts + "." + nonceHex + "." + blob
	mac := computeHMAC([]byte(hmacInput), secret)

	return challengeVersion + "." + ts + "." + nonceHex + "." + hex.EncodeToString(mac), nil
}

// VerifyChallenge checks the HMAC authenticity and timestamp freshness of a challenge.
func VerifyChallenge(challenge string, pubkey string, secret []byte, maxAge time.Duration, now time.Time) error {
	parts := strings.SplitN(challenge, ".", 4)
	if len(parts) != 4 {
		return ErrChallengeTampered
	}
	version, tsStr, nonceHex, macHex := parts[0], parts[1], parts[2], parts[3]

	if version != challengeVersion {
		return ErrChallengeTampered
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return ErrChallengeTampered
	}

	challengeTime := time.Unix(ts, 0)
	if now.Sub(challengeTime) > maxAge {
		return ErrChallengeExpired
	}
	if challengeTime.Sub(now) > maxClockSkew {
		return ErrChallengeFuture
	}

	blob, err := extractPubkeyBlob(pubkey)
	if err != nil {
		return ErrChallengeTampered
	}

	hmacInput := challengeVersion + "." + tsStr + "." + nonceHex + "." + blob
	expectedMAC := computeHMAC([]byte(hmacInput), secret)

	gotMAC, err := hex.DecodeString(macHex)
	if err != nil {
		return ErrChallengeTampered
	}

	if !hmac.Equal(gotMAC, expectedMAC) {
		return ErrChallengeTampered
	}

	return nil
}

// VerifySignature verifies an Ed25519 signature of the challenge string.
// Accepts two formats:
//   - Raw Ed25519 signature: base64-encoded 64-byte signature over the raw challenge bytes
//   - SSH signature (SSHSIG): base64-encoded binary blob from ssh-keygen -Y sign (namespace "edproof")
func VerifySignature(challenge string, pubkey string, signatureB64 string) error {
	rawKey, err := extractEd25519Key(pubkey)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}

	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return ErrSignatureInvalid
	}

	// Detect SSHSIG format (starts with "SSHSIG" magic)
	if bytes.HasPrefix(sig, []byte(sshsigMagic)) {
		return verifySSHSig(rawKey, []byte(challenge), sig)
	}

	// Raw Ed25519 signature
	if len(sig) != ed25519.SignatureSize {
		return ErrSignatureInvalid
	}
	if !ed25519.Verify(rawKey, []byte(challenge), sig) {
		return ErrSignatureInvalid
	}
	return nil
}

const sshsigMagic = "SSHSIG"
const sshsigNamespace = "edproof"

// verifySSHSig verifies an SSH signature in SSHSIG binary format.
// SSHSIG blob layout:
//
//	"SSHSIG" (6 bytes)
//	uint32 version (1)
//	string publickey (SSH wire format)
//	string namespace
//	string reserved
//	string hash_algorithm
//	string signature (SSH signature blob)
//
// The signed data is:
//
//	"SSHSIG" (6 bytes)
//	string namespace
//	string reserved (empty)
//	string hash_algorithm
//	string H(message)
func verifySSHSig(pubkey ed25519.PublicKey, message []byte, blob []byte) error {
	r := blob

	// Magic
	if len(r) < 6 || string(r[:6]) != sshsigMagic {
		return ErrSignatureInvalid
	}
	r = r[6:]

	// Version
	if len(r) < 4 {
		return ErrSignatureInvalid
	}
	version := binary.BigEndian.Uint32(r[:4])
	if version != 1 {
		return ErrSignatureInvalid
	}
	r = r[4:]

	// Public key (skip — we use the one from the request)
	r, err := skipSSHString(r)
	if err != nil {
		return ErrSignatureInvalid
	}

	// Namespace
	var namespace []byte
	namespace, r, err = readSSHString(r)
	if err != nil {
		return ErrSignatureInvalid
	}

	// Reserved (skip)
	r, err = skipSSHString(r)
	if err != nil {
		return ErrSignatureInvalid
	}

	// Hash algorithm
	var hashAlgo []byte
	hashAlgo, r, err = readSSHString(r)
	if err != nil {
		return ErrSignatureInvalid
	}

	// Signature blob (SSH signature wire format)
	var sigBlob []byte
	sigBlob, _, err = readSSHString(r)
	if err != nil {
		return ErrSignatureInvalid
	}

	// Extract raw Ed25519 signature from SSH signature blob
	// SSH signature blob: string key_type + string signature_data
	var keyType []byte
	keyType, sigBlob, err = readSSHString(sigBlob)
	if err != nil {
		return ErrSignatureInvalid
	}
	if string(keyType) != "ssh-ed25519" {
		return ErrSignatureInvalid
	}
	var rawSig []byte
	rawSig, _, err = readSSHString(sigBlob)
	if err != nil {
		return ErrSignatureInvalid
	}
	if len(rawSig) != ed25519.SignatureSize {
		return ErrSignatureInvalid
	}

	// Compute the hash of the message
	var messageHash []byte
	switch string(hashAlgo) {
	case "sha512":
		h := sha512.Sum512(message)
		messageHash = h[:]
	case "sha256":
		h := sha256.Sum256(message)
		messageHash = h[:]
	default:
		return ErrSignatureInvalid
	}

	// Reconstruct the signed data
	signedData := buildSSHSigSignedData(string(namespace), string(hashAlgo), messageHash)

	if !ed25519.Verify(pubkey, signedData, rawSig) {
		return ErrSignatureInvalid
	}
	return nil
}

// buildSSHSigSignedData constructs the data that was actually signed.
func buildSSHSigSignedData(namespace, hashAlgo string, messageHash []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(sshsigMagic)
	writeSSHString(&buf, []byte(namespace))
	writeSSHString(&buf, nil) // reserved (empty)
	writeSSHString(&buf, []byte(hashAlgo))
	writeSSHString(&buf, messageHash)
	return buf.Bytes()
}

func readSSHString(data []byte) (value []byte, rest []byte, err error) {
	if len(data) < 4 {
		return nil, nil, errors.New("short read")
	}
	length := binary.BigEndian.Uint32(data[:4])
	if uint32(len(data)-4) < length {
		return nil, nil, errors.New("truncated string")
	}
	return data[4 : 4+length], data[4+length:], nil
}

func skipSSHString(data []byte) (rest []byte, err error) {
	_, rest, err = readSSHString(data)
	return rest, err
}

func writeSSHString(buf *bytes.Buffer, data []byte) {
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	buf.Write(lenBuf)
	buf.Write(data)
}

// extractPubkeyBlob returns the base64 key blob (field 2) from an SSH public key line.
func extractPubkeyBlob(pubkey string) (string, error) {
	parts := strings.Fields(pubkey)
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid public key format")
	}
	if parts[0] != "ssh-ed25519" {
		return "", fmt.Errorf("unsupported key type %q", parts[0])
	}
	// Validate that the blob is valid base64
	if _, err := base64.StdEncoding.DecodeString(parts[1]); err != nil {
		return "", fmt.Errorf("invalid base64 in public key: %w", err)
	}
	return parts[1], nil
}

// extractEd25519Key parses an SSH public key line and returns the raw 32-byte Ed25519 public key.
// SSH wire format for ed25519: [4-byte len]["ssh-ed25519"][4-byte len][32-byte key]
func extractEd25519Key(pubkey string) (ed25519.PublicKey, error) {
	parts := strings.Fields(pubkey)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid public key format")
	}
	if parts[0] != "ssh-ed25519" {
		return nil, fmt.Errorf("unsupported key type %q", parts[0])
	}

	keyBlob, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %w", err)
	}

	// Parse SSH wire format
	if len(keyBlob) < 4 {
		return nil, fmt.Errorf("key blob too short")
	}
	typeLen := binary.BigEndian.Uint32(keyBlob[:4])
	if uint32(len(keyBlob)) < 4+typeLen+4 {
		return nil, fmt.Errorf("key blob too short for type")
	}
	keyType := string(keyBlob[4 : 4+typeLen])
	if keyType != "ssh-ed25519" {
		return nil, fmt.Errorf("wire format key type mismatch: %q", keyType)
	}

	rest := keyBlob[4+typeLen:]
	if len(rest) < 4 {
		return nil, fmt.Errorf("key blob missing key data length")
	}
	keyLen := binary.BigEndian.Uint32(rest[:4])
	if keyLen != ed25519.PublicKeySize {
		return nil, fmt.Errorf("unexpected ed25519 key size: %d", keyLen)
	}
	if uint32(len(rest)) < 4+keyLen {
		return nil, fmt.Errorf("key blob truncated")
	}

	return ed25519.PublicKey(rest[4 : 4+keyLen]), nil
}

func computeHMAC(message, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(message)
	return mac.Sum(nil)
}
