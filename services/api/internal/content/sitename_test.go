package content

import "testing"

func TestWithoutSiteName_DropsTheMirrorsNameOnly(t *testing.T) {
	t.Parallel()
	const freedium = "https://freedium-mirror.cfd/"
	for _, tc := range []struct{ title, site, want string }{
		{"How to Read PostgreSQL EXPLAIN - Freedium", freedium, "How to Read PostgreSQL EXPLAIN"},
		{"Top 7 Grafana Dashboards | freedium", freedium, "Top 7 Grafana Dashboards"},
		{"Queues - Mirror", freedium, "Queues - Mirror"},
		{"Queues - CFD", freedium, "Queues - CFD"},
		{"Queues - Example", "https://www.example.org/", "Queues"},
		{"Kafka - a guide - Freedium", freedium, "Kafka - a guide"},
		{"Freedium", freedium, "Freedium"},
		{"Queues - Freedium", "::bad", "Queues - Freedium"},
	} {
		if got := withoutSiteName(tc.title, tc.site); got != tc.want {
			t.Errorf("withoutSiteName(%q, %q) = %q, want %q", tc.title, tc.site, got, tc.want)
		}
	}
}
