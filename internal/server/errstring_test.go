package server

// errString is a string-typed error used by fakeDeployer scripts to avoid
// pulling in errors.New across every test case.
type errString string

func (e errString) Error() string { return string(e) }
