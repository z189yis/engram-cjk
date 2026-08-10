package store

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
)

// Phase 3 迁移测试：trigram 重建中断后可回滚、重启可重试（方案迁移矩阵）。

func TestMigrateInterruptedTrigramRebuildRollsBackAndRetries(t *testing.T) {
	s := newTestStore(t)
	t.Cleanup(func() { _ = s.Close() })

	if err := s.CreateSession("m1", "proj", "dir"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddObservation(AddObservationParams{
		SessionID: "m1", Type: "mem",
		Title: "迁移中断测试", Content: "数据必须保留", Project: "proj",
	}); err != nil {
		t.Fatal(err)
	}

	// 1. 手动把 FTS 表降级为旧 unicode61（模拟迁移前的状态）
	if _, err := s.execHook(s.db, `
		DROP TRIGGER IF EXISTS obs_fts_insert;
		DROP TRIGGER IF EXISTS obs_fts_update;
		DROP TRIGGER IF EXISTS obs_fts_delete;
		DROP TABLE IF EXISTS observations_fts;
		CREATE VIRTUAL TABLE observations_fts USING fts5(
			title,
			content,
			tool_name,
			type,
			project,
			topic_key,
			content='observations',
			content_rowid='id'
		);
		INSERT INTO observations_fts(rowid, title, content, tool_name, type, project, topic_key)
		SELECT id, title, content, tool_name, type, project, topic_key
		FROM observations
		WHERE deleted_at IS NULL;
	`); err != nil {
		t.Fatalf("downgrade fts: %v", err)
	}

	// 2. 在 trigram 重建的 INSERT 阶段注入失败（模拟中断）
	origExec := s.hooks.exec
	s.hooks.exec = func(db execer, query string, args ...any) (sql.Result, error) {
		if strings.Contains(query, "INSERT INTO observations_fts") {
			return nil, errors.New("forced interruption")
		}
		return origExec(db, query, args...)
	}
	err := s.migrate()
	s.hooks.exec = origExec
	if err == nil {
		t.Fatalf("expected migrate to fail on interruption")
	}

	// 3. 中断后：DDL（DROP/CREATE）在 SQLite 中无法回滚，但 INSERT 回滚保证
	//    不会出现"半填充"的损坏索引；下次 migrate 通过 ftsSQLUsesTrigram 检测
	//    会跳过重建（幂等），触发器重建保证索引可用
	var ftsSQL string
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='observations_fts'`).Scan(&ftsSQL); err != nil {
		t.Fatalf("fts table missing after rollback: %v", err)
	}
	t.Logf("回滚后 FTS schema: %.60s", ftsSQL)

	// 4. 重启可重试：再次 migrate 应成功（幂等，不 rebuild）
	if err := s.migrate(); err != nil {
		t.Fatalf("retry migrate: %v", err)
	}
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='observations_fts'`).Scan(&ftsSQL); err != nil {
		t.Fatalf("fts table after retry: %v", err)
	}
	if !strings.Contains(ftsSQL, "tokenize='trigram'") {
		t.Errorf("重试后 FTS 表应为 trigram，实际: %.80s", ftsSQL)
	}

	// 5. 数据完整性：观察记录完好
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE deleted_at IS NULL`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("迁移后基表应有 1 条记录，got %d", count)
	}

	// 6. 搜索可用（CJK 命中）
	results, err := s.Search("迁移中断测试", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("search after retry: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("重试后 CJK 搜索应命中 1 条，got %d", len(results))
	}
}
