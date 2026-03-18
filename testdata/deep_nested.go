package testdata

import (
	"errors"
	"fmt"
)

type Config struct {
	Debug   bool
	Timeout int
	Labels  map[string]string
}

func process(cfg Config, items []string) error {
	if cfg.Debug {
		fmt.Println("debug mode enabled")
	}

	for i, item := range items {
		if item == "" {
			continue
		}

		if cfg.Timeout > 0 {
			if i > cfg.Timeout {
				return errors.New("timeout exceeded")
			}

			if label, ok := cfg.Labels[item]; ok {
				if label == "skip" {
					continue
				} else if label == "stop" {
					return fmt.Errorf("stopped at item %q", item)
				} else {
					fmt.Printf("processing %q with label %q\n", item, label)
				}
			} else {
				fmt.Printf("processing %q (no label)\n", item)
			}
		}
	}

	return nil
}

func validate(cfg Config) []string {
	var errs []string

	if cfg.Timeout < 0 {
		errs = append(errs, "timeout must be non-negative")
	}

	for k, v := range cfg.Labels {
		if k == "" {
			errs = append(errs, "label key must not be empty")
		}
		if v == "" {
			errs = append(errs, fmt.Sprintf("label %q has empty value", k))
		}
	}

	return errs
}
