package ledger

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// Geçersiz kayıt, veritabanına dokunmadan reddedilir (nil tx ile kanıt).
func TestPostRejectsInvalidLines(t *testing.T) {
	for _, lines := range [][]Line{
		nil, {{AccountCode: "5000", DebitKurus: 100}},
		{{AccountCode: "5000", DebitKurus: -1}}, {{AccountCode: "5000", DebitKurus: 1, CreditKurus: 1}},
		{{AccountCode: "", DebitKurus: 1}}, {{AccountCode: "5000"}},
		{{AccountCode: "5000", DebitKurus: math.MaxInt64}, {AccountCode: "5000", DebitKurus: 1}},
	} {
		_, err := Post(context.Background(), nil, Entry{}, lines)
		require.Error(t, err)
	}
}
