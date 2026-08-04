package providers

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestReferenceRoundTrip(t *testing.T) {
	reference := Reference{
		Provider: ProviderCircleUserControlled, ChainID: testChainID, WalletID: testWalletID,
		ChallengeID: "challenge-1", ProviderTransactionID: "tx-1", TransactionHash: testHash,
	}
	encoded, err := reference.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "wzp1;") {
		t.Fatalf("encoding is not versioned: %q", encoded)
	}
	decoded, err := ParseReference(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != reference {
		t.Fatalf("round trip lost data: %+v", decoded)
	}
}

func TestReferenceEncodeOmitsEmptyFields(t *testing.T) {
	reference := Reference{Provider: ProviderCircleUserControlled, ChainID: testChainID, ChallengeID: "challenge-1"}
	encoded, err := reference.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if encoded != "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;ch=challenge-1" {
		t.Fatalf("unexpected encoding %q", encoded)
	}
}

func TestReferenceRequiresAnIdentifier(t *testing.T) {
	reference := Reference{Provider: ProviderCircleUserControlled, ChainID: testChainID, WalletID: testWalletID}
	if err := reference.Validate(); err == nil {
		t.Fatal("a reference with no provider identifier must be rejected")
	}
}

func TestParseReferenceFailsClosed(t *testing.T) {
	cases := map[string]string{
		"empty":                "",
		"unknown prefix":       "wzp2;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;ch=challenge-1",
		"no prefix":            "p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;ch=challenge-1",
		"unknown field":        "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;ch=challenge-1;secret=abc",
		"duplicate field":      "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;ch=a;ch=b",
		"empty value":          "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;ch=",
		"missing separator":    "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;challenge",
		"unknown provider":     "wzp1;p=other-provider;chain=5042002;ch=challenge-1",
		"invalid chain":        "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=05042002;ch=challenge-1",
		"non-numeric chain":    "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=arc;ch=challenge-1",
		"uppercase hash":       "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;hash=0x" + strings.Repeat("A", 64),
		"short hash":           "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;hash=0x1234",
		"no identifier":        "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002",
		"identifier with sign": "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;ch=chal;lenge",
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseReference(encoded); err == nil {
				t.Fatalf("%s must be rejected", name)
			}
		})
	}
}

// A provider identifier that could smuggle encoding separators would let a
// hostile provider response forge a different transaction hash on round trip.
func TestReferenceRejectsSeparatorsInIdentifiers(t *testing.T) {
	for _, identifier := range []string{"challenge;hash=0x" + strings.Repeat("1", 64), "chal=lenge", "chal lenge", strings.Repeat("c", 65)} {
		reference := Reference{Provider: ProviderCircleUserControlled, ChainID: testChainID, ChallengeID: identifier}
		if _, err := reference.Encode(); err == nil {
			t.Fatalf("identifier %q must be rejected", identifier)
		}
	}
}

func TestReferenceWithTransactionNeverClears(t *testing.T) {
	reference := Reference{
		Provider: ProviderCircleUserControlled, ChainID: testChainID,
		ChallengeID: "challenge-1", ProviderTransactionID: "tx-1", TransactionHash: testHash,
	}
	next := reference.WithTransaction("", "")
	if next != reference {
		t.Fatal("empty identifiers must not clear known ones")
	}
	next = reference.WithTransaction("tx-2", strings.ToUpper("0X")+strings.Repeat("A", 64))
	if next.ProviderTransactionID != "tx-2" {
		t.Fatalf("transaction ID not applied: %q", next.ProviderTransactionID)
	}
	if next.TransactionHash != "0x"+strings.Repeat("a", 64) {
		t.Fatalf("hash not normalized to lowercase: %q", next.TransactionHash)
	}
	if next.ChallengeID != "challenge-1" {
		t.Fatal("challenge ID must be preserved")
	}
}

func TestValidTransactionHash(t *testing.T) {
	valid := map[string]bool{
		testHash:                       true,
		"0x" + strings.Repeat("a", 64): true,
		"0x" + strings.Repeat("A", 64): false,
		"0x" + strings.Repeat("g", 64): false,
		strings.Repeat("a", 66):        false,
		"0x" + strings.Repeat("a", 63): false,
		"":                             false,
	}
	for value, expected := range valid {
		if ValidTransactionHash(value) != expected {
			t.Fatalf("ValidTransactionHash(%q) != %v", value, expected)
		}
	}
}

func TestValidAddressIsCaseInsensitive(t *testing.T) {
	if !ValidAddress("0x" + strings.Repeat("A", 40)) {
		t.Fatal("checksum casing is address metadata, not invalidity")
	}
	if ValidAddress("0x" + strings.Repeat("a", 41)) {
		t.Fatal("over-long address must be rejected")
	}
	if ValidAddress(strings.Repeat("a", 42)) {
		t.Fatal("address without 0x prefix must be rejected")
	}
}

func TestParseUintField(t *testing.T) {
	ok := []struct {
		in   string
		want uint64
	}{
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{strconv.FormatUint(math.MaxUint64, 10), math.MaxUint64},
	}
	for _, tt := range ok {
		got, err := parseUintField(tt.in)
		if err != nil || got != tt.want {
			t.Fatalf("parseUintField(%q) = %d, %v want %d", tt.in, got, err, tt.want)
		}
	}
	// MaxUint64 + 1 as decimal string.
	overflow := "18446744073709551616"
	// 20-digit value larger than MaxUint64.
	large20 := "99999999999999999999"
	reject := []string{"", "01", "00", "0x10", "1a", "-1", overflow, large20, " 1", "1 "}
	for _, in := range reject {
		if _, err := parseUintField(in); err == nil {
			t.Fatalf("parseUintField(%q) must fail closed", in)
		}
	}
}

func TestObservationMetadataRequiresTransactionHash(t *testing.T) {
	base := Reference{Provider: ProviderCircleUserControlled, ChainID: testChainID, ChallengeID: "challenge-1"}
	// Observation fields without a transaction hash must be rejected.
	malformed := base
	malformed.ObsKnown = true
	malformed.ObsBlockHash = testHash
	if err := malformed.Validate(); err == nil {
		t.Fatal("observation without transaction hash must be rejected")
	}
	encoded := "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;ch=challenge-1;rp=1;rk=1;bh=" + testHash
	if _, err := ParseReference(encoded); err == nil {
		t.Fatal("parsed observation without hash must be rejected")
	}
}

func TestObservationPresentImpliesKnownAndConfirmationsRequireKnown(t *testing.T) {
	// Present without known is normalized by helpers, but raw Validate rejects.
	raw := Reference{
		Provider: ProviderCircleUserControlled, ChainID: testChainID, TransactionHash: testHash,
		ObsPresent: true, ObsKnown: false,
	}
	if err := raw.Validate(); err == nil {
		t.Fatal("present without known must fail Validate")
	}
	// Confirmations alone without known observation.
	onlyCF := Reference{
		Provider: ProviderCircleUserControlled, ChainID: testChainID, TransactionHash: testHash,
		ObsConfirmations: 3,
	}
	if err := onlyCF.Validate(); err == nil {
		t.Fatal("confirmations without known observation must be rejected")
	}
	encoded := "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;hash=" + testHash + ";cf=3"
	if _, err := ParseReference(encoded); err == nil {
		t.Fatal("cf without known observation must fail parse")
	}
	// Present normalizes to known on WithReceiptObservation / Encode path.
	ok := referenceWithHash().WithReceiptObservation(ReceiptObservation{
		Present: true, BlockHash: testHash, BlockNumber: 9, Confirmations: 2, TransactionHash: testHash,
	})
	if !ok.ObsKnown || !ok.ObsPresent {
		t.Fatalf("normalized observation = %#v", ok)
	}
	if _, err := ok.Encode(); err != nil {
		t.Fatal(err)
	}
}

func TestObservationBlockIdentityNormalizesKnown(t *testing.T) {
	encoded := "wzp1;p=CIRCLE_USER_CONTROLLED_WALLET;chain=5042002;hash=" + testHash + ";bh=" + testHash + ";bn=10"
	decoded, err := ParseReference(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.ObsKnown || decoded.ObsBlockNumber != 10 || decoded.ObsBlockHash != testHash {
		t.Fatalf("decoded = %#v", decoded)
	}
}
