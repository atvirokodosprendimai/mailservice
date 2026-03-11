package edproof

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("test-secret-must-be-at-least-32-bytes-long!!")

// makeTestSSHPubkey creates an SSH public key line from a raw ed25519 public key.
func makeTestSSHPubkey(pub ed25519.PublicKey) string {
	// SSH wire format: [4-byte len]["ssh-ed25519"][4-byte len][32-byte key]
	keyType := "ssh-ed25519"
	blob := make([]byte, 0, 4+len(keyType)+4+len(pub))
	typeLenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(typeLenBuf, uint32(len(keyType)))
	blob = append(blob, typeLenBuf...)
	blob = append(blob, keyType...)
	keyLenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(keyLenBuf, uint32(len(pub)))
	blob = append(blob, keyLenBuf...)
	blob = append(blob, pub...)
	return "ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + " test@test"
}

func TestGenerateChallenge(t *testing.T) {
	t.Parallel()

	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubkey := makeTestSSHPubkey(pub)
	now := time.Now().UTC()

	challenge, err := GenerateChallenge(pubkey, testSecret, now)
	if err != nil {
		t.Fatalf("GenerateChallenge: %v", err)
	}

	parts := strings.SplitN(challenge, ".", 4)
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d", len(parts))
	}
	if parts[0] != "v1" {
		t.Fatalf("expected v1 prefix, got %q", parts[0])
	}
	// Nonce should be 32 hex chars (16 bytes)
	if len(parts[2]) != 32 {
		t.Fatalf("expected 32 hex char nonce, got %d", len(parts[2]))
	}
	// HMAC should be 64 hex chars (32 bytes)
	if len(parts[3]) != 64 {
		t.Fatalf("expected 64 hex char HMAC, got %d", len(parts[3]))
	}
}

func TestGenerateChallengeRejectsInvalidKey(t *testing.T) {
	t.Parallel()

	_, err := GenerateChallenge("not-a-key", testSecret, time.Now())
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestVerifyChallengeHappyPath(t *testing.T) {
	t.Parallel()

	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubkey := makeTestSSHPubkey(pub)
	now := time.Now().UTC()

	challenge, err := GenerateChallenge(pubkey, testSecret, now)
	if err != nil {
		t.Fatal(err)
	}

	err = VerifyChallenge(challenge, pubkey, testSecret, 30*time.Second, now)
	if err != nil {
		t.Fatalf("VerifyChallenge: %v", err)
	}
}

func TestVerifyChallengeExpired(t *testing.T) {
	t.Parallel()

	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubkey := makeTestSSHPubkey(pub)
	past := time.Now().UTC().Add(-60 * time.Second)

	challenge, err := GenerateChallenge(pubkey, testSecret, past)
	if err != nil {
		t.Fatal(err)
	}

	err = VerifyChallenge(challenge, pubkey, testSecret, 30*time.Second, time.Now().UTC())
	if err != ErrChallengeExpired {
		t.Fatalf("expected ErrChallengeExpired, got %v", err)
	}
}

func TestVerifyChallengeTampered(t *testing.T) {
	t.Parallel()

	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubkey := makeTestSSHPubkey(pub)
	now := time.Now().UTC()

	challenge, err := GenerateChallenge(pubkey, testSecret, now)
	if err != nil {
		t.Fatal(err)
	}

	// Tamper with the HMAC
	tampered := challenge[:len(challenge)-2] + "ff"
	err = VerifyChallenge(tampered, pubkey, testSecret, 30*time.Second, now)
	if err != ErrChallengeTampered {
		t.Fatalf("expected ErrChallengeTampered, got %v", err)
	}
}

func TestVerifyChallengeWrongPubkey(t *testing.T) {
	t.Parallel()

	pub1, _, _ := ed25519.GenerateKey(nil)
	pub2, _, _ := ed25519.GenerateKey(nil)
	pubkey1 := makeTestSSHPubkey(pub1)
	pubkey2 := makeTestSSHPubkey(pub2)
	now := time.Now().UTC()

	challenge, err := GenerateChallenge(pubkey1, testSecret, now)
	if err != nil {
		t.Fatal(err)
	}

	// Verify with different pubkey — HMAC won't match
	err = VerifyChallenge(challenge, pubkey2, testSecret, 30*time.Second, now)
	if err != ErrChallengeTampered {
		t.Fatalf("expected ErrChallengeTampered, got %v", err)
	}
}

func TestVerifyChallengeFutureTimestamp(t *testing.T) {
	t.Parallel()

	pub, _, _ := ed25519.GenerateKey(nil)
	pubkey := makeTestSSHPubkey(pub)
	future := time.Now().UTC().Add(30 * time.Second)

	challenge, err := GenerateChallenge(pubkey, testSecret, future)
	if err != nil {
		t.Fatal(err)
	}

	err = VerifyChallenge(challenge, pubkey, testSecret, 30*time.Second, time.Now().UTC())
	if err != ErrChallengeFuture {
		t.Fatalf("expected ErrChallengeFuture, got %v", err)
	}
}

func TestVerifyChallengeMalformedInputs(t *testing.T) {
	t.Parallel()

	pub, _, _ := ed25519.GenerateKey(nil)
	pubkey := makeTestSSHPubkey(pub)
	now := time.Now().UTC()

	tests := []struct {
		name      string
		challenge string
	}{
		{"empty", ""},
		{"no dots", "v1abcdef"},
		{"two parts", "v1.123"},
		{"three parts", "v1.123.abc"},
		{"wrong version", "v2.123.abc.def"},
		{"non-numeric ts", "v1.notanumber.abc.def"},
		{"invalid hmac hex", "v1.123.abc.zzzz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyChallenge(tt.challenge, pubkey, testSecret, 30*time.Second, now)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestVerifySignatureHappyPath(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubkey := makeTestSSHPubkey(pub)
	now := time.Now().UTC()

	challenge, err := GenerateChallenge(pubkey, testSecret, now)
	if err != nil {
		t.Fatal(err)
	}

	sig := ed25519.Sign(priv, []byte(challenge))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	err = VerifySignature(challenge, pubkey, sigB64)
	if err != nil {
		t.Fatalf("VerifySignature: %v", err)
	}
}

func TestVerifySignatureWrongKey(t *testing.T) {
	t.Parallel()

	pub1, _, _ := ed25519.GenerateKey(nil)
	_, priv2, _ := ed25519.GenerateKey(nil)
	pubkey1 := makeTestSSHPubkey(pub1)

	challenge := "v1.123456.abcdef.000000"
	sig := ed25519.Sign(priv2, []byte(challenge))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	err := VerifySignature(challenge, pubkey1, sigB64)
	if err != ErrSignatureInvalid {
		t.Fatalf("expected ErrSignatureInvalid, got %v", err)
	}
}

func TestVerifySignatureWrongMessage(t *testing.T) {
	t.Parallel()

	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeTestSSHPubkey(pub)

	sig := ed25519.Sign(priv, []byte("wrong message"))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	err := VerifySignature("actual challenge", pubkey, sigB64)
	if err != ErrSignatureInvalid {
		t.Fatalf("expected ErrSignatureInvalid, got %v", err)
	}
}

func TestVerifySignatureMalformedInputs(t *testing.T) {
	t.Parallel()

	pub, _, _ := ed25519.GenerateKey(nil)
	pubkey := makeTestSSHPubkey(pub)

	tests := []struct {
		name string
		sig  string
	}{
		{"not base64", "!!!not-base64!!!"},
		{"too short", base64.StdEncoding.EncodeToString([]byte("short"))},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifySignature("some challenge", pubkey, tt.sig)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestExtractEd25519Key(t *testing.T) {
	t.Parallel()

	pub, _, _ := ed25519.GenerateKey(nil)
	pubkey := makeTestSSHPubkey(pub)

	extracted, err := extractEd25519Key(pubkey)
	if err != nil {
		t.Fatalf("extractEd25519Key: %v", err)
	}

	if !extracted.Equal(pub) {
		t.Fatal("extracted key does not match original")
	}
}

func TestExtractPubkeyBlob(t *testing.T) {
	t.Parallel()

	pub, _, _ := ed25519.GenerateKey(nil)
	pubkey := makeTestSSHPubkey(pub)
	parts := strings.Fields(pubkey)

	blob, err := extractPubkeyBlob(pubkey)
	if err != nil {
		t.Fatalf("extractPubkeyBlob: %v", err)
	}
	if blob != parts[1] {
		t.Fatalf("expected %q, got %q", parts[1], blob)
	}
}

func TestExtractPubkeyBlobIgnoresComment(t *testing.T) {
	t.Parallel()

	pub, _, _ := ed25519.GenerateKey(nil)
	pubkey1 := makeTestSSHPubkey(pub)
	// Same key, different comment
	parts := strings.Fields(pubkey1)
	pubkey2 := parts[0] + " " + parts[1] + " different@comment"

	blob1, _ := extractPubkeyBlob(pubkey1)
	blob2, _ := extractPubkeyBlob(pubkey2)

	if blob1 != blob2 {
		t.Fatalf("comment should not affect blob: %q vs %q", blob1, blob2)
	}
}

func TestFullChallengeResponseFlow(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubkey := makeTestSSHPubkey(pub)
	now := time.Now().UTC()

	// 1. Generate challenge
	challenge, err := GenerateChallenge(pubkey, testSecret, now)
	if err != nil {
		t.Fatalf("GenerateChallenge: %v", err)
	}

	// 2. Client signs challenge
	sig := ed25519.Sign(priv, []byte(challenge))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	// 3. Verify challenge authenticity
	err = VerifyChallenge(challenge, pubkey, testSecret, 30*time.Second, now)
	if err != nil {
		t.Fatalf("VerifyChallenge: %v", err)
	}

	// 4. Verify signature
	err = VerifySignature(challenge, pubkey, sigB64)
	if err != nil {
		t.Fatalf("VerifySignature: %v", err)
	}

	// 5. Extract fingerprint
	fingerprint, err := FingerprintFromPubkey(pubkey)
	if err != nil {
		t.Fatalf("FingerprintFromPubkey: %v", err)
	}
	if !strings.HasPrefix(fingerprint, "sha256:") {
		t.Fatalf("expected sha256: prefix, got %q", fingerprint)
	}
}

func TestUniqueNonces(t *testing.T) {
	t.Parallel()

	pub, _, _ := ed25519.GenerateKey(nil)
	pubkey := makeTestSSHPubkey(pub)
	now := time.Now().UTC()

	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		challenge, err := GenerateChallenge(pubkey, testSecret, now)
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.SplitN(challenge, ".", 4)
		nonce := parts[2]
		if seen[nonce] {
			t.Fatalf("duplicate nonce on iteration %d", i)
		}
		seen[nonce] = true
	}
}

// buildTestSSHSig creates an SSHSIG binary blob for testing.
func buildTestSSHSig(pub ed25519.PublicKey, priv ed25519.PrivateKey, message []byte, namespace string) []byte {
	// Compute message hash
	h := sha512.Sum512(message)

	// Build signed data
	var signedData bytes.Buffer
	signedData.WriteString("SSHSIG")
	writeTestSSHString(&signedData, []byte(namespace))
	writeTestSSHString(&signedData, nil) // reserved
	writeTestSSHString(&signedData, []byte("sha512"))
	writeTestSSHString(&signedData, h[:])

	// Sign it
	sig := ed25519.Sign(priv, signedData.Bytes())

	// Build SSH public key wire format
	var pubKeyBlob bytes.Buffer
	writeTestSSHString(&pubKeyBlob, []byte("ssh-ed25519"))
	writeTestSSHString(&pubKeyBlob, pub)

	// Build SSH signature blob (inside the SSHSIG)
	var sigBlob bytes.Buffer
	writeTestSSHString(&sigBlob, []byte("ssh-ed25519"))
	writeTestSSHString(&sigBlob, sig)

	// Build SSHSIG blob
	var blob bytes.Buffer
	blob.WriteString("SSHSIG")
	versionBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(versionBuf, 1)
	blob.Write(versionBuf)
	writeTestSSHString(&blob, pubKeyBlob.Bytes())
	writeTestSSHString(&blob, []byte(namespace))
	writeTestSSHString(&blob, nil) // reserved
	writeTestSSHString(&blob, []byte("sha512"))
	writeTestSSHString(&blob, sigBlob.Bytes())

	return blob.Bytes()
}

func writeTestSSHString(buf *bytes.Buffer, data []byte) {
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	buf.Write(lenBuf)
	buf.Write(data)
}

func TestVerifySignatureSSHSIGFormat(t *testing.T) {
	t.Parallel()

	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeTestSSHPubkey(pub)
	now := time.Now().UTC()

	challenge, _ := GenerateChallenge(pubkey, testSecret, now)

	sshsig := buildTestSSHSig(pub, priv, []byte(challenge), "edproof")
	sigB64 := base64.StdEncoding.EncodeToString(sshsig)

	err := VerifySignature(challenge, pubkey, sigB64)
	if err != nil {
		t.Fatalf("VerifySignature with SSHSIG: %v", err)
	}
}

func TestVerifySignatureSSHSIGWrongKey(t *testing.T) {
	t.Parallel()

	pub1, _, _ := ed25519.GenerateKey(nil)
	_, priv2, _ := ed25519.GenerateKey(nil)
	pubkey1 := makeTestSSHPubkey(pub1)

	challenge := "test-challenge"
	// Sign with wrong private key but claim pub1
	sshsig := buildTestSSHSig(pub1, priv2, []byte(challenge), "edproof")
	sigB64 := base64.StdEncoding.EncodeToString(sshsig)

	err := VerifySignature(challenge, pubkey1, sigB64)
	if err != ErrSignatureInvalid {
		t.Fatalf("expected ErrSignatureInvalid, got %v", err)
	}
}

func TestVerifySignatureSSHSIGAnyNamespace(t *testing.T) {
	t.Parallel()

	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeTestSSHPubkey(pub)
	challenge := "test-challenge"

	// Should accept any namespace (the server doesn't enforce a specific one)
	sshsig := buildTestSSHSig(pub, priv, []byte(challenge), "file")
	sigB64 := base64.StdEncoding.EncodeToString(sshsig)

	err := VerifySignature(challenge, pubkey, sigB64)
	if err != nil {
		t.Fatalf("VerifySignature with namespace 'file': %v", err)
	}
}

func TestFullFlowWithSSHSIG(t *testing.T) {
	t.Parallel()

	pub, priv, _ := ed25519.GenerateKey(nil)
	pubkey := makeTestSSHPubkey(pub)
	now := time.Now().UTC()

	// 1. Generate challenge
	challenge, err := GenerateChallenge(pubkey, testSecret, now)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Client signs with ssh-keygen style (SSHSIG)
	sshsig := buildTestSSHSig(pub, priv, []byte(challenge), "edproof")
	sigB64 := base64.StdEncoding.EncodeToString(sshsig)

	// 3. Verify challenge
	err = VerifyChallenge(challenge, pubkey, testSecret, 30*time.Second, now)
	if err != nil {
		t.Fatal(err)
	}

	// 4. Verify signature
	err = VerifySignature(challenge, pubkey, sigB64)
	if err != nil {
		t.Fatal(err)
	}

	// 5. Fingerprint
	fp, _ := FingerprintFromPubkey(pubkey)
	if !strings.HasPrefix(fp, "sha256:") {
		t.Fatalf("bad fingerprint: %s", fp)
	}
}

// TestWithRealSSHKey tests with the known test key from the existing verifier_test.go
func TestWithRealSSHKey(t *testing.T) {
	t.Parallel()

	pubkey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOB0H114bTlib+M0AuEoXJDWHzU52aMKtT8O1wtpk5WB entity@context"
	now := time.Now().UTC()

	// Should be able to generate a challenge
	challenge, err := GenerateChallenge(pubkey, testSecret, now)
	if err != nil {
		t.Fatalf("GenerateChallenge: %v", err)
	}

	// Should verify
	err = VerifyChallenge(challenge, pubkey, testSecret, 30*time.Second, now)
	if err != nil {
		t.Fatalf("VerifyChallenge: %v", err)
	}

	// Should extract ed25519 key
	rawKey, err := extractEd25519Key(pubkey)
	if err != nil {
		t.Fatalf("extractEd25519Key: %v", err)
	}
	if len(rawKey) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(rawKey))
	}

	// Key should match what we expect from the base64 blob
	expectedKeyHex := hex.EncodeToString(rawKey)
	t.Logf("extracted ed25519 key: %s", expectedKeyHex)
}
