package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// alertsAPI answers every tool call with result and records the arguments of the
// last one.
func alertsAPI(t *testing.T, result string) (*httptest.Server, *map[string]any) {
	t.Helper()
	args := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req struct {
			Params struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		json.Unmarshal(b, &req)
		args = req.Params.Arguments
		res, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": result}}})
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":" + string(res) + "}\n\n"))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("FREEHIRE_TOKEN", "good")
	t.Setenv("FREEHIRE_API_URL", srv.URL)
	return srv, &args
}

const alertResult = `{"alert":{"id":7,"name":"Go EU","filter":"skills: go","paused":false,"channels":[{"channel":"email"}]}}`

func TestAlertsAdd_SendsQueryFiltersAndSalary(t *testing.T) {
	_, args := alertsAPI(t, alertResult)
	out, err := run(t, "alerts", "add", "backend", "--skills", "go", "--remote", "--salary-min", "5000", "--channel", "telegram")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	a := *args
	filters, _ := a["filters"].(map[string]any)
	if a["query"] != "backend" || filters["skills"] == nil || filters["work_mode"] == nil {
		t.Errorf("args = %v, want the query and the skills/work_mode filters", a)
	}
	if _, inFilters := filters["salary_min"]; inFilters || a["salary_min"] != float64(5000) {
		t.Errorf("args = %v, want salary_min beside the filters, not in them", a)
	}
	if !strings.Contains(out, "#7  Go EU") {
		t.Errorf("out = %q", out)
	}
}

func TestAlertsUpdate_SendsOnlyWhatChanged(t *testing.T) {
	// A sent part REPLACES the alert's, so a rename must not send filters: an empty
	// filter set would clear every one the alert has.
	_, args := alertsAPI(t, alertResult)
	if _, err := run(t, "alerts", "update", "7", "--name", "New"); err != nil {
		t.Fatalf("update: %v", err)
	}
	a := *args
	if _, ok := a["filters"]; ok {
		t.Errorf("args = %v, want no filters on a rename", a)
	}
	if a["name"] != "New" || a["id"] != float64(7) {
		t.Errorf("args = %v, want id and name", a)
	}
}

func TestAlertsUpdate_ClearFiltersSendsAnEmptySet(t *testing.T) {
	_, args := alertsAPI(t, alertResult)
	if _, err := run(t, "alerts", "update", "7", "--clear-filters"); err != nil {
		t.Fatalf("update: %v", err)
	}
	filters, ok := (*args)["filters"].(map[string]any)
	if !ok || len(filters) != 0 {
		t.Errorf("args = %v, want an empty filters object", *args)
	}
}

func TestAlertsUpdate_RefusesAnEmptyChange(t *testing.T) {
	alertsAPI(t, alertResult)
	if _, err := run(t, "alerts", "update", "7"); err == nil || !strings.Contains(err.Error(), "nothing to change") {
		t.Errorf("err = %v, want a usage refusal before any call", err)
	}
}

func TestAlertsPause_SetsPaused(t *testing.T) {
	_, args := alertsAPI(t, alertResult)
	if _, err := run(t, "alerts", "pause", "7"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if (*args)["paused"] != true {
		t.Errorf("args = %v, want paused=true", *args)
	}
}

func TestAlertsAdd_WarnsOnDroppedValues(t *testing.T) {
	alertsAPI(t, `{"alert":{"id":7,"name":"x"},"ignored_params":["skills=golangg"]}`)
	_, stderr, err := runSplit(t, "alerts", "add", "--skills", "golangg", "--skills", "go")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(stderr, "skills=golangg") {
		t.Errorf("stderr = %q, want the dropped value named", stderr)
	}
}
