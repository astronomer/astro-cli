package deploy

// newestOf is the newest of builds, newest first as BuildsFor gives them, or
// "" for none.
func newestOf(builds []string) string {
	if len(builds) == 0 {
		return ""
	}
	return builds[0]
}
