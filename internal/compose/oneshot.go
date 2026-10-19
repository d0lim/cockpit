package compose

func OneShots(config map[string]any) map[string]bool {
	services, _ := config["services"].(map[string]any)
	out := map[string]bool{}
	for name, raw := range services {
		svc, _ := raw.(map[string]any)
		if restart, _ := svc["restart"].(string); restart == "no" {
			out[name] = true
		}
		for dep, cond := range dependsOn(svc) {
			if _, defined := services[dep]; defined && cond == "service_completed_successfully" {
				out[dep] = true
			}
		}
	}
	return out
}
