package store

import "testing"

// Phase 3 触发器测试：软删除门控 + 恢复重索引 + 硬删除清理。
// 验证方案 §7 Phase 3 的 insert/update/hard delete/soft delete/restore 行为。

func TestTriggersSoftDeleteExcludesFromFTSTriggerIndex(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	if err := s.CreateSession("t1", "proj", "dir"); err != nil {
		t.Fatal(err)
	}

	// 1. 插入活记录 → 触发 obs_fts_insert（WHEN new.deleted_at IS NULL）→ 索引
	liveID, err := s.AddObservation(AddObservationParams{
		SessionID: "t1", Type: "mem",
		Title: "trigram 触发器验证", Content: "soft delete test", Project: "proj",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 2. 直接以 deleted_at 非空插入（模拟导入已删行）→ 触发器不索引
	if _, err := s.execHook(s.db, `
		INSERT INTO observations (sync_id, session_id, type, title, content, project, scope, deleted_at)
		VALUES ('obs-dead-1', 't1', 'mem', '已删除记录', 'deleted content', 'proj', 'project', datetime('now'))
	`); err != nil {
		t.Fatalf("insert deleted row: %v", err)
	}

	// 3. 软删除活记录 → 触发器应删除索引行
	if err := s.DeleteObservation(liveID, false); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	// 4. 验证索引中不存在软删除行（直接查 FTS 表，绕过查询层过滤）
	count := 0
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM observations_fts fts
		JOIN observations o ON o.id = fts.rowid
		WHERE observations_fts MATCH 'trigram' AND o.deleted_at IS NOT NULL
	`).Scan(&count); err != nil {
		t.Fatalf("count fts soft-deleted: %v", err)
	}
	if count != 0 {
		t.Errorf("FTS 索引中仍存在 %d 条软删除行（触发器未删除索引）", count)
	}

	// 5. 恢复（restore）：deleted_at 清空 → update 触发器自动重新索引
	if _, err := s.execHook(s.db,
		`UPDATE observations SET deleted_at = NULL, updated_at = datetime('now') WHERE id = ?`,
		liveID); err != nil {
		t.Fatalf("restore: %v", err)
	}

	// 6. 验证恢复后索引包含该行
	restoreCount := 0
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM observations_fts fts
		JOIN observations o ON o.id = fts.rowid
		WHERE observations_fts MATCH 'trigram' AND o.deleted_at IS NULL
	`).Scan(&restoreCount); err != nil {
		t.Fatalf("count fts restored: %v", err)
	}
	if restoreCount != 1 {
		t.Errorf("恢复后 FTS 索引应有 1 条活记录，got %d", restoreCount)
	}

	// 7. 硬删除 → delete 触发器删除索引行
	if err := s.DeleteObservation(liveID, true); err != nil {
		t.Fatalf("hard delete: %v", err)
	}
	hardCount := 0
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM observations_fts
		WHERE rowid = ?`, liveID).Scan(&hardCount); err != nil {
		t.Fatalf("count fts after hard delete: %v", err)
	}
	if hardCount != 0 {
		t.Errorf("硬删除后 FTS 索引仍有 %d 条残留", hardCount)
	}
}

// 重复重建触发器（迁移幂等性）：触发器带 WHEN 条件后仍应可重复创建。
func TestTriggersRecreateIdempotentWithWhenClause(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	for i := 0; i < 3; i++ {
		if err := s.recreateObservationFTSTriggers(); err != nil {
			t.Fatalf("recreate observation triggers #%d: %v", i, err)
		}
		if err := s.recreatePromptFTSTriggers(); err != nil {
			t.Fatalf("recreate prompt triggers #%d: %v", i, err)
		}
	}

	// 重建后触发器仍生效
	if err := s.CreateSession("t2", "proj", "dir"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddObservation(AddObservationParams{
		SessionID: "t2", Type: "mem",
		Title: "重建后触发器验证", Content: "still works", Project: "proj",
	}); err != nil {
		t.Fatal(err)
	}
	results, err := s.Search("重建后触发器验证", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("search after recreate: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("重建触发器后搜索应命中 1 条，got %d", len(results))
	}
}
