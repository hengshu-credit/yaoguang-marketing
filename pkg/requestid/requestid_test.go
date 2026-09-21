package requestid

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestEnsureReusesGeneratedRequestID(t *testing.T) {
	r := &http.Request{}
	id := Ensure(r)
	_, err := uuid.Parse(id)
	require.NoError(t, err)
	require.Equal(t, id, Ensure(r))
	require.NotEqual(t, id, Ensure(nil))
}
