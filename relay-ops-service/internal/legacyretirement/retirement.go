package legacyretirement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
)

const ConfirmationPhrase = "DELETE_WITHOUT_BACKUP"

var Tables = []string{
	"agent_analyses",
	"notification_deliveries",
	"incidents",
	"notification_messages",
	"notification_decisions",
	"group_impact_signals",
	"operational_baselines",
	"native_ops_alert_events",
	"native_ops_alert_sync_state",
}

type Database interface {
	Count(context.Context, string) (exists bool, rows int64, err error)
	Retire(context.Context, []string) error
}

type Options struct {
	Execute      bool
	Confirmation string
}

type TableReport struct {
	Name   string `json:"name"`
	Exists bool   `json:"exists"`
	Rows   int64  `json:"rows"`
}

type Report struct {
	Mode   string        `json:"mode"`
	Tables []TableReport `json:"tables"`
}

func Run(ctx context.Context, database Database, options Options, output io.Writer) error {
	if database == nil || output == nil {
		return errors.New("retirement dependencies are unavailable")
	}
	if options.Execute && options.Confirmation != ConfirmationPhrase {
		return errors.New("retirement confirmation is missing")
	}
	report := Report{Mode: "count_only", Tables: make([]TableReport, 0, len(Tables))}
	for _, table := range Tables {
		exists, rows, err := database.Count(ctx, table)
		if err != nil {
			return fmt.Errorf("count legacy notification table: %w", err)
		}
		report.Tables = append(report.Tables, TableReport{Name: table, Exists: exists, Rows: rows})
	}
	if options.Execute {
		if err := database.Retire(ctx, append([]string(nil), Tables...)); err != nil {
			return fmt.Errorf("retire legacy notification tables: %w", err)
		}
		report.Mode = "executed"
	}
	return json.NewEncoder(output).Encode(report)
}

func authorizedOrder(tables []string) bool {
	return reflect.DeepEqual(tables, Tables)
}
