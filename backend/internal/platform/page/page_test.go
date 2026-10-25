package page

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	p, bad := Parse(httptest.NewRequest("GET", "/x", nil))
	assert.Nil(t, bad)
	assert.Equal(t, Params{Limit: DefaultLimit}, p)

	p, bad = Parse(httptest.NewRequest("GET", "/x?limit=7&cursor="+Encode(42), nil))
	assert.Nil(t, bad)
	assert.Equal(t, Params{Limit: 7, AfterID: 42}, p)

	for _, q := range []string{"limit=0", "limit=101", "limit=abc", "cursor=bozuk", "cursor=" + Encode(0), "cursor=NDI"} {
		_, bad = Parse(httptest.NewRequest("GET", "/x?"+q, nil))
		assert.NotNil(t, bad, q)
	}
}

func TestBuild(t *testing.T) {
	id := func(v int64) int64 { return v }
	r := Build([]int64{5, 4}, 2, id)
	if assert.NotNil(t, r.NextCursor) {
		p, _ := Parse(httptest.NewRequest("GET", "/x?cursor="+*r.NextCursor, nil))
		assert.Equal(t, int64(4), p.AfterID)
	}
	assert.Nil(t, Build([]int64{5}, 2, id).NextCursor, "son sayfa")
	assert.NotNil(t, Build[int64](nil, 2, id).Items, "boş liste [] olarak yazılır")
}
