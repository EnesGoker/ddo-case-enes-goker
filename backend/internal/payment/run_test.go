package payment

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestRunDistinguishesSystemAndPolicyErrors(t *testing.T) {
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, io.EOF, io.ErrUnexpectedEOF, io.ErrClosedPipe, net.ErrClosed, &pgconn.PgError{Code: "08006"}, &pgconn.PgError{Code: "53100"}, &pgconn.PgError{Code: "57P01"}, &pgconn.PgError{Code: "58030"}, &pgconn.PgError{Code: "XX000"}} {
		assert.True(t, runSystemError(context.Background(), err), err)
	}
	for _, err := range []error{errors.New("yalnız bu poliçede hata"), &pgconn.PgError{Code: "23514"}, &pgconn.PgError{Code: "40001"}} {
		assert.False(t, runSystemError(context.Background(), err), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.True(t, runSystemError(ctx, errors.New("işlem hatası")))
}
