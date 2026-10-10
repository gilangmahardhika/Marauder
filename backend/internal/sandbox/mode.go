package sandbox

func AllowOff(mode string, fixture bool) bool {
	return mode == "off" && fixture
}
