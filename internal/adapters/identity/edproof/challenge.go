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

	"github.com/atvirokodosprendimai/mailservice/internal/core/ports"
)

// Authenticator implements ports.ChallengeAuthenticator using Ed25519 + HMAC-SHA256.
type Authenticator struct {
	secret []byte
}

// NewAuthenticator creates a ChallengeAuthenticator backed by the given HMAC secret.
func NewAuthenticator(secret []byte) *Authenticator {
	return &Authenticator{secret: secret}
}

func (a *Authenticator) GenerateChallenge(pubkey string, now time.Time) (string, error) {
	return GenerateChallenge(pubkey, a.secret, now)
}

func (a *Authenticator) VerifyChallenge(challenge, pubkey string, maxAge time.Duration, now time.Time) error {
	return verifyChallengeWithSecret(challenge, pubkey, a.secret, maxAge, now)
}

func (a *Authenticator) VerifySignature(challenge, pubkey, signature string) error {
	return VerifySignature(challenge, pubkey, signature)
}

func (a *Authenticator) FingerprintFromPubkey(pubkey string) (string, error) {
	return FingerprintFromPubkey(pubkey)
}

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
// Deprecated: Use Authenticator.VerifyChallenge instead.
func VerifyChallenge(challenge string, pubkey string, secret []byte, maxAge time.Duration, now time.Time) error {
	return verifyChallengeWithSecret(challenge, pubkey, secret, maxAge, now)
}

func verifyChallengeWithSecret(challenge string, pubkey string, secret []byte, maxAge time.Duration, now time.Time) error {
	parts := strings.SplitN(challenge, ".", 4)
	if len(parts) != 4 {
		return ports.ErrChallengeTampered
	}
	version, tsStr, nonceHex, macHex := parts[0], parts[1], parts[2], parts[3]

	if version != challengeVersion {
		return ports.ErrChallengeTampered
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return ports.ErrChallengeTampered
	}

	challengeTime := time.Unix(ts, 0)
	if now.Sub(challengeTime) > maxAge {
		return ports.ErrChallengeExpired
	}
	if challengeTime.Sub(now) > maxClockSkew {
		return ports.ErrChallengeFuture
	}

	blob, err := extractPubkeyBlob(pubkey)
	if err != nil {
		return ports.ErrChallengeTampered
	}

	hmacInput := challengeVersion + "." + tsStr + "." + nonceHex + "." + blob
	expectedMAC := computeHMAC([]byte(hmacInput), secret)

	gotMAC, err := hex.DecodeString(macHex)
	if err != nil {
		return ports.ErrChallengeTampered
	}

	if !hmac.Equal(gotMAC, expectedMAC) {
		return ports.ErrChallengeTampered
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

	if len(signatureB64) > 1024 {
		return ports.ErrSignatureInvalid
	}

	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return ports.ErrSignatureInvalid
	}

	// Detect SSHSIG format (starts with "SSHSIG" magic)
	if bytes.HasPrefix(sig, []byte(sshsigMagic)) {
		return verifySSHSig(rawKey, []byte(challenge), sig)
	}

	// Raw Ed25519 signature
	if len(sig) != ed25519.SignatureSize {
		return ports.ErrSignatureInvalid
	}
	if !ed25519.Verify(rawKey, []byte(challenge), sig) {
		return ports.ErrSignatureInvalid
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
		return ports.ErrSignatureInvalid
	}
	r = r[6:]

	// Version
	if len(r) < 4 {
		return ports.ErrSignatureInvalid
	}
	version := binary.BigEndian.Uint32(r[:4])
	if version != 1 {
		return ports.ErrSignatureInvalid
	}
	r = r[4:]

	// Public key (skip — we use the one from the request)
	r, err := skipSSHString(r)
	if err != nil {
		return ports.ErrSignatureInvalid
	}

	// Namespace
	var namespace []byte
	namespace, r, err = readSSHString(r)
	if err != nil {
		return ports.ErrSignatureInvalid
	}
	if string(namespace) != sshsigNamespace {
		return ports.ErrSignatureInvalid
	}

	// Reserved (skip)
	r, err = skipSSHString(r)
	if err != nil {
		return ports.ErrSignatureInvalid
	}

	// Hash algorithm
	var hashAlgo []byte
	hashAlgo, r, err = readSSHString(r)
	if err != nil {
		return ports.ErrSignatureInvalid
	}

	// Signature blob (SSH signature wire format)
	var sigBlob []byte
	sigBlob, _, err = readSSHString(r)
	if err != nil {
		return ports.ErrSignatureInvalid
	}

	// Extract raw Ed25519 signature from SSH signature blob
	// SSH signature blob: string key_type + string signature_data
	var keyType []byte
	keyType, sigBlob, err = readSSHString(sigBlob)
	if err != nil {
		return ports.ErrSignatureInvalid
	}
	if string(keyType) != "ssh-ed25519" {
		return ports.ErrSignatureInvalid
	}
	var rawSig []byte
	rawSig, _, err = readSSHString(sigBlob)
	if err != nil {
		return ports.ErrSignatureInvalid
	}
	if len(rawSig) != ed25519.SignatureSize {
		return ports.ErrSignatureInvalid
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
		return ports.ErrSignatureInvalid
	}

	// Reconstruct the signed data
	signedData := buildSSHSigSignedData(string(namespace), string(hashAlgo), messageHash)

	if !ed25519.Verify(pubkey, signedData, rawSig) {
		return ports.ErrSignatureInvalid
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
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	buf.Write(lenBuf[:])
	buf.Write(data)
}

// parseSSHPubkey validates an SSH public key line and returns the decoded key blob.
// Input format: "ssh-ed25519 <base64> [comment]"
func parseSSHPubkey(pubkey string) ([]byte, error) {
	parts := strings.Fields(pubkey)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid public key format")
	}
	if parts[0] != "ssh-ed25519" {
		return nil, fmt.Errorf("unsupported key type %q", parts[0])
	}
	blob, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid base64 in public key: %w", err)
	}
	return blob, nil
}

// extractPubkeyBlob returns the base64 key blob (field 2) from an SSH public key line.
func extractPubkeyBlob(pubkey string) (string, error) {
	if _, err := parseSSHPubkey(pubkey); err != nil {
		return "", err
	}
	return strings.Fields(pubkey)[1], nil
}

// extractEd25519Key parses an SSH public key line and returns the raw 32-byte Ed25519 public key.
// SSH wire format for ed25519: [4-byte len]["ssh-ed25519"][4-byte len][32-byte key]
func extractEd25519Key(pubkey string) (ed25519.PublicKey, error) {
	keyBlob, err := parseSSHPubkey(pubkey)
	if err != nil {
		return nil, err
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
