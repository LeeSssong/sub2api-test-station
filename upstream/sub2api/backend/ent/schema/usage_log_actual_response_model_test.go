package schema

import (
	"testing"

	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/require"
)

func TestUsageLogActualResponseModelIsNullableAndCapped(t *testing.T) {
	spec, err := (&load.Config{Path: "."}).Load()
	require.NoError(t, err)

	var usageLog *load.Schema
	for _, candidate := range spec.Schemas {
		if candidate.Name == "UsageLog" {
			usageLog = candidate
			break
		}
	}
	require.NotNil(t, usageLog)
	actual := requireSchemaField(t, usageLog, "actual_response_model")
	require.Equal(t, field.TypeString, actual.Info.Type)
	require.True(t, actual.Optional)
	require.True(t, actual.Nillable)
	require.NotNil(t, actual.Size)
	require.Equal(t, int64(100), *actual.Size)
}
