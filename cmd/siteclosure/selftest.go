package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type selfClient struct {
	base     string
	client   *http.Client
	revision int64
}

func runSelftest(base string) error {
	c := &selfClient{base: base, client: &http.Client{Timeout: 4 * time.Second}}
	caseID := "selftest-case"
	type mutation struct {
		Case struct {
			Revision int64 `json:"revision"`
			Plan     *struct {
				PlanDigest string `json:"plan_digest"`
				Review     struct {
					Risks []struct {
						Code string `json:"code"`
					} `json:"risks"`
				} `json:"review"`
			} `json:"plan"`
		} `json:"case"`
	}
	var out mutation
	if err := c.post("/api/cases", map[string]any{"request_id": "st-create", "case_id": caseID, "site_code": "SELFTEST", "trench_coordinates": "N1/E1", "completion_record_digest": "sha256:selftest-record", "exposed_surface_condition": "稳定", "created_by": "recorder"}, &out); err != nil {
		return err
	}
	var precheck struct {
		SummaryDigest string `json:"summary_digest"`
		CanFreeze     bool   `json:"can_freeze"`
	}
	if err := c.post("/api/cases/"+caseID+"/baseline/precheck", map[string]any{"actor": "recorder", "people": []string{"recorder", "worker"}}, &precheck); err != nil {
		return err
	}
	if !precheck.CanFreeze {
		return fmt.Errorf("baseline precheck failed")
	}
	if err := c.post("/api/cases/"+caseID+"/baseline", map[string]any{"request_id": "st-baseline", "actor": "recorder", "people": []string{"recorder", "worker"}, "confirmed_digest": precheck.SummaryDigest, "expected_revision": out.Case.Revision}, &out); err != nil {
		return err
	}
	layers := []map[string]any{{"index": 1, "material_code": "clean-soil", "target_thickness_mm": 200, "thickness_tolerance_mm": 10, "moisture_min_percent": 12, "moisture_max_percent": 18, "compaction_min_percent": 90, "evidence_required": true}, {"index": 2, "material_code": "protective-soil", "target_thickness_mm": 150, "thickness_tolerance_mm": 8, "moisture_min_percent": 10, "moisture_max_percent": 16, "compaction_min_percent": 88, "evidence_required": true}}
	if err := c.post("/api/cases/"+caseID+"/plan", map[string]any{"request_id": "st-plan", "prepared_by": "planner", "expected_revision": out.Case.Revision, "layers": layers}, &out); err != nil {
		return err
	}
	risks := make([]string, 0, len(out.Case.Plan.Review.Risks))
	for _, risk := range out.Case.Plan.Review.Risks {
		risks = append(risks, risk.Code)
	}
	if err := c.post("/api/cases/"+caseID+"/approve", map[string]any{"request_id": "st-approve", "actor": "approver", "expected_revision": out.Case.Revision, "plan_digest": out.Case.Plan.PlanDigest, "confirmed_risk_codes": risks}, &out); err != nil {
		return err
	}
	if err := c.post("/api/cases/"+caseID+"/layers", map[string]any{"request_id": "st-layer-1", "execution_id": "exec-1", "layer_index": 1, "material_code": "clean-soil", "actual_thickness_mm": 201, "moisture_percent": 15, "compaction_percent": 92, "performed_by": "worker", "evidence_digest": "sha256:evidence-1", "expected_revision": out.Case.Revision}, &out); err != nil {
		return err
	}
	if err := c.post("/api/cases/"+caseID+"/layers", map[string]any{"request_id": "st-layer-2", "execution_id": "exec-2", "layer_index": 2, "material_code": "protective-soil", "actual_thickness_mm": 150, "moisture_percent": 13, "compaction_percent": 90, "performed_by": "worker", "evidence_digest": "sha256:evidence-2", "expected_revision": out.Case.Revision}, &out); err != nil {
		return err
	}
	if err := c.post("/api/cases/"+caseID+"/review", map[string]any{"request_id": "st-review", "actor": "independent-reviewer", "decision": "pass", "issues": []any{}, "expected_revision": out.Case.Revision}, &out); err != nil {
		return err
	}
	var verify struct {
		Valid         bool   `json:"valid"`
		DossierDigest string `json:"dossier_digest"`
	}
	if err := c.get("/api/cases/"+caseID+"/verify", &verify); err != nil {
		return err
	}
	if !verify.Valid || verify.DossierDigest == "" {
		return fmt.Errorf("dossier verification failed")
	}
	var dossier map[string]any
	if err := c.get("/api/cases/"+caseID+"/dossier", &dossier); err != nil {
		return err
	}
	fmt.Printf("selftest passed: case=%s digest=%s\n", caseID, verify.DossierDigest)
	return nil
}
func (c *selfClient) post(path string, body any, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}
func (c *selfClient) get(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}
func (c *selfClient) do(req *http.Request, out any) error {
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s %s: status=%d body=%s", req.Method, req.URL.Path, res.StatusCode, string(b))
	}
	if out != nil && len(b) > 0 {
		return json.Unmarshal(b, out)
	}
	return nil
}
