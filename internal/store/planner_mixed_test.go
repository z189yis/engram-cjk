package store

import "testing"

// Phase 2 规划器测试：验证混合长度查询按词规划而非整体降级。
// 这些测试锁定的行为是方案 §5.1-§5.3 的验收标准。

func TestPlannerMixedQueryRecall(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	for _, sid := range []string{"s1", "s2", "s3", "s4"} {
		if err := s.CreateSession(sid, "proj", "dir"); err != nil {
			t.Fatalf("create session %s: %v", sid, err)
		}
	}

	add := func(title, content string) int64 {
		t.Helper()
		id, err := s.AddObservation(AddObservationParams{
			SessionID: "s1", Type: "mem",
			Title: title, Content: content, Project: "proj",
		})
		if err != nil {
			t.Fatalf("add obs: %v", err)
		}
		return id
	}

	// 语料：每条都包含 1 个短词 + 1 个长词，验证混合查询不丢短词
	v2MigrationID := add("v2 migration guide", "how to migrate to v2")
	dbOptID := add("数据库 优化", "SQLite 连接池调优")
	sqliteModelID := add("SQLite 模型", "v2 训练方案")
	tokyoModelID := add("東京都 model", "実行する方法")

	cases := []struct {
		name   string
		query  string
		mode   string
		wantID int64
	}{
		{"v2 migration all", "v2 migration", "all", v2MigrationID},
		{"数据库 优化 all", "数据库 优化", "all", dbOptID},
		{"SQLite 模型 all", "SQLite 模型", "all", sqliteModelID},
		{"東京都 model all", "東京都 model", "all", tokyoModelID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results, err := s.Search(tc.query, SearchOptions{MatchMode: tc.mode, Limit: 10})
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			found := false
			for _, r := range results {
				if r.ID == tc.wantID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("查询 %q 未命中期望记录 #%d，got %d 条", tc.query, tc.wantID, len(results))
			}
		})
	}

	// any 模式：短词应扩大召回（任一命中即返回）
	anyResults, err := s.Search("migration 优化", SearchOptions{MatchMode: "any", Limit: 10})
	if err != nil {
		t.Fatalf("any search: %v", err)
	}
	if len(anyResults) < 2 {
		t.Errorf("any 模式混合查询应召回 >= 2 条，got %d", len(anyResults))
	}
}

// 混合查询中短词的 LIKE 转义：字面 % 不应被当作通配符。
func TestPlannerMixedQueryLiteralPercent(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	if err := s.CreateSession("p1", "proj", "dir"); err != nil {
		t.Fatal(err)
	}
	literalID, err := s.AddObservation(AddObservationParams{
		SessionID: "p1", Type: "mem",
		Title: "进度 100%", Content: "done migration", Project: "proj",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherID, err := s.AddObservation(AddObservationParams{
		SessionID: "p1", Type: "mem",
		Title: "任意文本", Content: "anything migration", Project: "proj",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 混合查询 "100% migration"：短词 "100%" 应匹配字面字符，仅命中 literalID
	results, err := s.Search("100% migration", SearchOptions{MatchMode: "all", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	for _, r := range results {
		if r.ID == otherID {
			t.Errorf("混合查询 %q 误命中 #%d（%% 被当作通配符）", "100% migration", otherID)
		}
	}
	found := false
	for _, r := range results {
		if r.ID == literalID {
			found = true
		}
	}
	if !found {
		t.Errorf("混合查询 %q 未命中字面 %% 记录 #%d", "100% migration", literalID)
	}
}

// 混合查询（FTS + 短词 LIKE）必须排除软删除记录。
func TestPlannerMixedQueryExcludesSoftDeleted(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	if err := s.CreateSession("p2", "proj", "dir"); err != nil {
		t.Fatal(err)
	}
	liveID, err := s.AddObservation(AddObservationParams{
		SessionID: "p2", Type: "mem",
		Title: "migration 优化", Content: "live record", Project: "proj",
	})
	if err != nil {
		t.Fatal(err)
	}
	deletedID, err := s.AddObservation(AddObservationParams{
		SessionID: "p2", Type: "mem",
		Title: "migration 优化", Content: "deleted record", Project: "proj",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteObservation(deletedID, false); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	results, err := s.Search("migration 优化", SearchOptions{MatchMode: "all", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	for _, r := range results {
		if r.ID == deletedID {
			t.Errorf("软删除记录 #%d 出现在混合查询结果中", deletedID)
		}
		if r.ID == liveID {
			t.Logf("活记录 #%d 正常命中", liveID)
		}
	}
	if len(results) == 0 {
		t.Errorf("混合查询未命中任何活记录")
	}
}
