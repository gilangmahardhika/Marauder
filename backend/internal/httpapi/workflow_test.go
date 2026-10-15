package httpapi_test

import (
	"strings"
	"testing"
)

func TestWorkflowSettings(t *testing.T) {
	f := newDeployFixture(t)
	path := "/api/projects/" + f.projectA + "/settings/workflow"
	got := getAuth(t, f.srv, path, f.dev)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"implementationCap":2`) || !strings.Contains(got.Body.String(), `"soloApproval":false`) {
		t.Fatalf("defaults = %d %s", got.Code, got.Body.String())
	}
	saved := putJSON(t, f.srv, path, f.lead, map[string]any{"implementationCap": 4, "soloApproval": true})
	if saved.Code != 200 || !strings.Contains(saved.Body.String(), `"implementationCap":4`) || !strings.Contains(saved.Body.String(), `"soloApproval":true`) {
		t.Fatalf("put = %d %s", saved.Code, saved.Body.String())
	}
	bad := putJSON(t, f.srv, path, f.lead, map[string]any{"implementationCap": 9})
	if bad.Code != 422 || !strings.Contains(bad.Body.String(), `"implementationCap"`) {
		t.Fatalf("cap 9 = %d %s", bad.Code, bad.Body.String())
	}
	if denied := putJSON(t, f.srv, path, f.dev, map[string]any{"soloApproval": false}); denied.Code != 403 {
		t.Fatalf("developer put = %d %s", denied.Code, denied.Body.String())
	}
	kept := putJSON(t, f.srv, path, f.lead, map[string]any{"implementationCap": 3})
	if !strings.Contains(kept.Body.String(), `"soloApproval":true`) {
		t.Fatalf("a save without soloApproval changed it: %s", kept.Body.String())
	}
	other := getAuth(t, f.srv, "/api/projects/"+f.projectB+"/settings/workflow", f.lead)
	if !strings.Contains(other.Body.String(), `"implementationCap":2`) {
		t.Fatalf("project B saw A's workflow: %s", other.Body.String())
	}
	deploy := putJSON(t, f.srv, "/api/projects/"+f.projectA+"/settings/deploy", f.lead, map[string]any{"environment": "dev"})
	if deploy.Code != 200 || strings.Contains(deploy.Body.String(), "implementationCap") || strings.Contains(deploy.Body.String(), "soloApproval") || strings.Contains(deploy.Body.String(), "prdPath") {
		t.Fatalf("deploy = %d %s", deploy.Code, deploy.Body.String())
	}
	after := getAuth(t, f.srv, path, f.lead)
	if !strings.Contains(after.Body.String(), `"implementationCap":3`) || !strings.Contains(after.Body.String(), `"soloApproval":true`) {
		t.Fatalf("deploy save reset the workflow: %s", after.Body.String())
	}
}

func TestRepositoryDocPaths(t *testing.T) {
	f := newDeployFixture(t)
	path := "/api/projects/" + f.projectA + "/settings/repository"
	got := getAuth(t, f.srv, path, f.dev)
	if got.Code != 200 || !strings.Contains(got.Body.String(), `"prdPath":"docs/prd"`) || !strings.Contains(got.Body.String(), `"rfcPath":"docs/rfc"`) {
		t.Fatalf("defaults = %d %s", got.Code, got.Body.String())
	}
	saved := putJSON(t, f.srv, path, f.lead, map[string]any{"repo": "widget", "baseBranch": "main", "prdPath": "specs/prd", "rfcPath": "specs/rfc"})
	if saved.Code != 200 || !strings.Contains(saved.Body.String(), `"prdPath":"specs/prd"`) || !strings.Contains(saved.Body.String(), `"rfcPath":"specs/rfc"`) {
		t.Fatalf("put = %d %s", saved.Code, saved.Body.String())
	}
	bad := putJSON(t, f.srv, path, f.lead, map[string]any{"repo": "widget", "baseBranch": "develop", "prdPath": "../outside"})
	if bad.Code != 422 || !strings.Contains(bad.Body.String(), `"prdPath"`) {
		t.Fatalf("bad path = %d %s", bad.Code, bad.Body.String())
	}
	if after := getAuth(t, f.srv, path, f.lead); !strings.Contains(after.Body.String(), `"baseBranch":"main"`) {
		t.Fatalf("a bad path still saved the repository: %s", after.Body.String())
	}
	kept := putJSON(t, f.srv, path, f.lead, map[string]any{"repo": "widget", "baseBranch": "main"})
	if kept.Code != 200 || !strings.Contains(kept.Body.String(), `"prdPath":"specs/prd"`) {
		t.Fatalf("a save without paths changed them: %d %s", kept.Code, kept.Body.String())
	}
	other := getAuth(t, f.srv, "/api/projects/"+f.projectB+"/settings/repository", f.lead)
	if !strings.Contains(other.Body.String(), `"prdPath":"docs/prd"`) {
		t.Fatalf("project B saw A's paths: %s", other.Body.String())
	}
}
