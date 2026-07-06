package match

import "testing"

func TestScore(t *testing.T) {
	t.Parallel()
	in := Input{
		ChartName:   "ingress-nginx",
		ChartRepo:   "https://kubernetes.github.io/ingress-nginx",
		Home:        "https://github.com/kubernetes/ingress-nginx",
		Maintainers: []string{"Kubernetes SIG Network"},
	}
	perfect := Package{
		Name:          "ingress-nginx",
		Normalized:    "ingress-nginx",
		RepositoryURL: "https://kubernetes.github.io/ingress-nginx/",
		HomeURL:       "https://github.com/kubernetes/ingress-nginx",
		Maintainers:   []string{"Kubernetes SIG Network"},
	}
	if got := Score(in, perfect); got < 0.85 {
		t.Errorf("perfect match score = %v, want ≥ 0.85", got)
	}

	nameOnly := Package{Name: "ingress-nginx", Normalized: "ingress-nginx"}
	if got := Score(in, nameOnly); got != 0.4 {
		t.Errorf("name-only score = %v, want 0.4", got)
	}

	wrong := Package{Name: "nginx-ingress", Normalized: "nginx-ingress"}
	if got := Score(in, wrong); got != 0 {
		t.Errorf("wrong name score = %v, want 0", got)
	}
}

func TestBest(t *testing.T) {
	t.Parallel()
	in := Input{ChartName: "prometheus", ChartRepo: "https://prometheus-community.github.io/helm-charts"}
	pkgs := []Package{
		{Name: "kube-prometheus-stack", Normalized: "kube-prometheus-stack", RepositoryURL: "https://prometheus-community.github.io/helm-charts"},
		{Name: "prometheus", Normalized: "prometheus", RepositoryURL: "https://prometheus-community.github.io/helm-charts"},
		{Name: "prometheus", Normalized: "prometheus", RepositoryURL: "https://charts.bitnami.com/bitnami"},
	}
	got, conf, ok := Best(in, pkgs)
	if !ok {
		t.Fatal("expected a match")
	}
	if got.Normalized != "prometheus" || conf < 0.7 {
		t.Errorf("Best() = %+v conf=%v, want prometheus with conf ≥ 0.7", got, conf)
	}
	if got.RepositoryURL != "https://prometheus-community.github.io/helm-charts" {
		t.Errorf("repo = %q", got.RepositoryURL)
	}
}

func TestURLsMatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://charts.example.com/", "https://charts.example.com", true},
		{"http://Charts.Example.COM/path", "http://charts.example.com/path/", true},
		{"https://a.example.com", "https://b.example.com", false},
	}
	for _, tc := range cases {
		if got := urlsMatch(tc.a, tc.b); got != tc.want {
			t.Errorf("urlsMatch(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
