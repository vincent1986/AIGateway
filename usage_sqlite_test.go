package main

import (
	"testing"
)

func TestUsageSQLiteRecordAndStats(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	closeDB()
	defer closeDB()

	list := []Provider{
		{
			ID: "p1", Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", APIKey: "sk",
			Models: []ProviderModel{{ID: "deepseek-chat", Enabled: true}},
		},
	}
	if err := saveProvidersToDisk(list); err != nil {
		t.Fatal(err)
	}

	recordUsage("DeepSeek", "deepseek-chat", "chat/completions", 200, 10, 20, 30)
	recordUsage("DeepSeek", "deepseek-chat", "chat/completions", 200, 5, 5, 10)

	st := (&App{}).GetUsageStats()
	if st.Total.Calls != 2 {
		t.Fatalf("calls=%d", st.Total.Calls)
	}
	if st.Total.TotalTokens != 40 {
		t.Fatalf("tokens=%d", st.Total.TotalTokens)
	}

	// reorder routes
	groups, err := (&App{}).ListModelGroups()
	if err != nil || len(groups) == 0 {
		t.Fatalf("groups %v %v", groups, err)
	}
	g := groups[0]
	ids := make([]string, 0, len(g.Routes))
	for _, r := range g.Routes {
		ids = append(ids, r.ID)
	}
	if len(ids) > 0 {
		// reverse if more than one; single is fine
		if err := (&App{}).ReorderModelGroupRoutes(g.ID, ids); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := (&App{}).ClearUsageStats(); err != nil {
		t.Fatal(err)
	}
	st = (&App{}).GetUsageStats()
	if st.Total.Calls != 0 {
		t.Fatalf("after clear calls=%d", st.Total.Calls)
	}
}
