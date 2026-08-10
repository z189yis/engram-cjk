package store

import "testing"

// 隔离验证 any 混合模式的并集召回：
// 记录 A 只含长词 "migration"，记录 B 只含短词 "优化"。
// any 模式查 "migration 优化" 应同时召回 A 和 B（并集语义）。

func TestPlannerAnyMixedUnionRecall(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	if err := s.CreateSession("u1", "proj", "dir"); err != nil {
		t.Fatal(err)
	}
	longOnly, err := s.AddObservation(AddObservationParams{
		SessionID: "u1", Type: "mem",
		Title: "migration notes", Content: "general", Project: "proj",
	})
	if err != nil {
		t.Fatal(err)
	}
	shortOnly, err := s.AddObservation(AddObservationParams{
		SessionID: "u1", Type: "mem",
		Title: "优化方案", Content: "某项目", Project: "proj",
	})
	if err != nil {
		t.Fatal(err)
	}

	results, err := s.Search("migration 优化", SearchOptions{MatchMode: "any", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	seen := map[int64]bool{}
	for _, r := range results {
		seen[r.ID] = true
		t.Logf("any 召回 #%d %q", r.ID, r.Title)
	}
	if !seen[longOnly] {
		t.Errorf("any 模式未召回仅含长词记录 #%d", longOnly)
	}
	if !seen[shortOnly] {
		t.Errorf("any 模式未召回仅含短词记录 #%d", shortOnly)
	}
}
