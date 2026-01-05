package pathwatcher

func GetWatchPaths(rootPaths []string) ([]string, error) {
	return getWatchPaths(rootPaths)
}
