package extensiontests

func (results ExtensionTestResults) Walk(walkFn func(*ExtensionTestResult)) {
	for i := range results {
		walkFn(results[i])
	}
}
