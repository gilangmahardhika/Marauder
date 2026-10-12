package router

import (
	"errors"
	"fmt"
)

func TaskClass(risk string, attemptsUsed int) (string, error) {
	if risk != "low" && risk != "high" {
		return "", fmt.Errorf("risk %q is invalid", risk)
	}
	if risk == "high" || attemptsUsed >= 1 {
		return "implement_hard", nil
	}
	return "implement", nil
}

func RouteModel(model string) error {
	if model == "" {
		return fmt.Errorf("router_config")
	}
	return nil
}

func RejectDeploy(class string) error {
	if class == "deploy" {
		return fmt.Errorf("task class deploy is not allowed")
	}
	return nil
}

type Family struct {
	ID     string
	Models []string
}

func ReviewModel(authorFamily string, families []Family) (model string, independent bool) {
	var same string
	for _, family := range families {
		if len(family.Models) == 0 {
			continue
		}
		if family.ID != authorFamily {
			return family.Models[0], true
		}
		if same == "" {
			same = family.Models[0]
		}
	}
	return same, false
}

type Caller func(model string) error

// ErrNoRetry wraps an error that Call returns at once, without the retry or
// the fallback, such as a stream that failed after its first text.
type ErrNoRetry struct{ Err error }

func (e ErrNoRetry) Error() string { return e.Err.Error() }
func (e ErrNoRetry) Unwrap() error { return e.Err }

func Call(primary, fallback string, call Caller) error {
	err := call(primary)
	if err == nil {
		return nil
	}
	if errors.As(err, new(ErrNoRetry)) {
		return fmt.Errorf("provider_error: %w", err)
	}
	if err2 := call(primary); err2 == nil {
		return nil
	} else if errors.As(err2, new(ErrNoRetry)) {
		return fmt.Errorf("provider_error: %w", err2)
	} else {
		err = err2
	}
	if fallback == "" {
		return fmt.Errorf("provider_error: %s", err.Error())
	}
	if err2 := call(fallback); err2 == nil {
		return nil
	} else {
		return fmt.Errorf("provider_error: %s; %s", err.Error(), err2.Error())
	}
}
