package store

import (
	"fmt"
	"testing"
)

// Phase 4 性能优化回归测试：候选物化（过采样 + 自适应扩大）不丢召回。
// 覆盖：项目隔离、软删除、超限候选（候选数 > limit×10）下的召回完整性。

// TestCandidateMaterializationRecall 验证过采样截断后 Recall 完整。
// 构造 300 条含关键词的记录（limit=10，候选上限 100 会被截断），
// 自适应扩大候选集后应仍能召回全部 300 条中的前 10 条。
func TestCandidateMaterializationRecall(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	if err := s.CreateSession("cm1", "proj", "dir"); err != nil {
		t.Fatal(err)
	}
	// 300 条含关键词的记录（超过初始候选上限 100）
	ids := make(map[int64]bool, 300)
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		res, err := tx.Exec(`
			INSERT INTO observations (sync_id, session_id, type, title, content, project, scope, normalized_hash, revision_count, duplicate_count, last_seen_at, created_at, updated_at)
			VALUES (?, 'cm1', 'mem', ?, ?, 'proj', 'project', ?, 1, 1, datetime('now'), datetime('now'), datetime('now'))
		`, fmt.Sprintf("cm-%d", i), fmt.Sprintf("候选测试 %d", i), fmt.Sprintf("数据库连接池优化记录 %d", i), hashNormalized(fmt.Sprintf("数据库连接池优化记录 %d", i)))
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		id, _ := res.LastInsertId()
		ids[id] = true
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// 搜索 limit=10：候选上限 100 被截断 → 自适应扩大后应召回
	results, err := s.Search("数据库连接池", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 10 {
		t.Errorf("应返回 10 条，got %d", len(results))
	}
	// 全部结果应在 ids 中（无无关记录）
	for _, r := range results {
		if !ids[r.ID] {
			t.Errorf("结果 #%d 不在期望集合中", r.ID)
		}
	}
}

// TestCandidateMaterializationProjectIsolation 验证项目过滤在候选截断后仍正确。
func TestCandidateMaterializationProjectIsolation(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	if err := s.CreateSession("cm2", "proj-a", "dir"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession("cm3", "proj-b", "dir"); err != nil {
		t.Fatal(err)
	}

	// 项目 A：1 条含关键词
	aID, err := s.AddObservation(AddObservationParams{
		SessionID: "cm2", Type: "mem",
		Title: "数据库连接池", Content: "项目 A 记录", Project: "proj-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 项目 B：200 条含关键词（会把候选集全部占满）
	tx, _ := s.db.Begin()
	for i := 0; i < 200; i++ {
		tx.Exec(`
			INSERT INTO observations (sync_id, session_id, type, title, content, project, scope, normalized_hash, revision_count, duplicate_count, last_seen_at, created_at, updated_at)
			VALUES (?, 'cm3', 'mem', ?, ?, 'proj-b', 'project', ?, 1, 1, datetime('now'), datetime('now'), datetime('now'))
		`, fmt.Sprintf("b-%d", i), fmt.Sprintf("项目B记录%d", i), fmt.Sprintf("数据库连接池配置 %d", i), hashNormalized(fmt.Sprintf("数据库连接池配置 %d", i)))
	}
	tx.Commit()

	// 搜索项目 A：候选集被项目 B 占满（bm25 排序），A 的记录必须仍可召回
	results, err := s.Search("数据库连接池", SearchOptions{Project: "proj-a", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range results {
		if r.ID == aID {
			found = true
		}
	}
	if !found {
		t.Errorf("项目 A 记录 #%d 未被召回（候选截断导致项目隔离召回缺失）", aID)
	}
}

// TestCandidateMaterializationSoftDelete 验证软删除行在候选截断后仍被排除。
func TestCandidateMaterializationSoftDelete(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	if err := s.CreateSession("cm4", "proj", "dir"); err != nil {
		t.Fatal(err)
	}
	// 200 条含关键词，其中一半软删除
	tx, _ := s.db.Begin()
	for i := 0; i < 200; i++ {
		// 偶数 i 直接插入为已删除状态
		sqlText := `
			INSERT INTO observations (sync_id, session_id, type, title, content, project, scope, normalized_hash, revision_count, duplicate_count, last_seen_at, created_at, updated_at)
			VALUES (?, 'cm4', 'mem', ?, ?, 'proj', 'project', ?, 1, 1, datetime('now'), datetime('now'), datetime('now'))
		`
		if _, err := tx.Exec(sqlText,
			fmt.Sprintf("sd-%d", i), fmt.Sprintf("软删测试%d", i), fmt.Sprintf("数据库连接池记录 %d", i), hashNormalized(fmt.Sprintf("数据库连接池记录 %d", i))); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	tx.Commit()

	// 软删除一半记录（偶数行）
	for i := 0; i < 200; i += 2 {
		if _, err := s.execHook(s.db,
			`UPDATE observations SET deleted_at = '2026-01-01 00:00:00' WHERE sync_id = ?`,
			fmt.Sprintf("sd-%d", i)); err != nil {
			t.Fatalf("soft delete %d: %v", i, err)
		}
	}

	// 搜索：软删除行不得出现在结果中
	results, err := s.Search("数据库连接池", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.DeletedAt != nil {
			t.Errorf("软删除记录 #%d 出现在候选物化搜索结果中", r.ID)
		}
	}
}

// TestCandidateMaterializationHighFrequency 高频词（100% 命中率）下仍能返回正确数量。
func TestCandidateMaterializationHighFrequency(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	if err := s.CreateSession("cm5", "proj", "dir"); err != nil {
		t.Fatal(err)
	}
	const n = 10000
	tx, _ := s.db.Begin()
	for i := 0; i < n; i++ {
		tx.Exec(`
			INSERT INTO observations (sync_id, session_id, type, title, content, project, scope, normalized_hash, revision_count, duplicate_count, last_seen_at, created_at, updated_at)
			VALUES (?, 'cm5', 'mem', ?, ?, 'proj', 'project', ?, 1, 1, datetime('now'), datetime('now'), datetime('now'))
		`, fmt.Sprintf("hf-%d", i), fmt.Sprintf("高频%d", i), fmt.Sprintf("数据库连接池优化 %d", i), hashNormalized(fmt.Sprintf("数据库连接池优化 %d", i)))
	}
	tx.Commit()

	// limit=20：候选上限 200，10k 全命中 → 自适应扩大 4 轮后应返回 20 条
	results, err := s.Search("数据库连接池", SearchOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 20 {
		t.Errorf("高频词 limit=20 应返回 20 条，got %d", len(results))
	}
}
