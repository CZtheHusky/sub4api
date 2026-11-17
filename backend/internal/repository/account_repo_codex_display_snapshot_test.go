package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/MACOS-DO/sub4api/ent"
	"github.com/MACOS-DO/sub4api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateExtraCodexSnapshotsSchedulerOutbox(t *testing.T) {
	// codex_credits_snapshot 是 credits 接管豁免（openAICreditOverageActive）的调度
	// 输入：写入必须发出 scheduler_outbox 事件并重建投影，否则 has_credits 耗尽
	// 对候选过滤不可见，账号会在额度打满后持续放行并反复 429。
	// codex_referral_snapshot 仍是纯展示快照：只做 UPDATE，不进事务、不发事件。
	for _, tc := range []struct {
		key             string
		schedulingInput bool
	}{
		{key: "codex_credits_snapshot", schedulingInput: true},
		{key: "codex_referral_snapshot", schedulingInput: false},
	} {
		for _, scenario := range []struct {
			name             string
			value            any
			schedulingChange bool
		}{
			{"refresh", map[string]any{"fetched_at": 1770000000}, false},
			{"invalidate", nil, false},
			{"mixed scheduling update", nil, true},
		} {
			t.Run(tc.key+"/"+scenario.name, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				require.NoError(t, err)
				client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
				t.Cleanup(func() { _ = client.Close() })
				repo := newAccountRepositoryWithSQL(client, db, nil)
				updates := map[string]any{tc.key: scenario.value}
				if scenario.schedulingChange {
					updates["openai_compact_enabled"] = true
				}
				expectOutbox := scenario.schedulingChange || tc.schedulingInput
				if expectOutbox {
					mock.ExpectBegin()
				}
				payload, err := json.Marshal(updates)
				require.NoError(t, err)
				mock.ExpectExec(regexp.QuoteMeta("UPDATE accounts SET extra = COALESCE(extra, '{}'::jsonb) || $1::jsonb, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL")).
					WithArgs(string(payload), int64(27)).WillReturnResult(sqlmock.NewResult(0, 1))
				if expectOutbox {
					mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
						WithArgs(service.SchedulerOutboxEventAccountChanged, int64(27), nil, nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
					mock.ExpectCommit()
				}
				require.NoError(t, repo.UpdateExtra(context.Background(), 27, updates))
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}
