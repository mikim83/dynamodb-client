// Command dynamodb-client is a terminal UI to browse DynamoDB tables with the
// profiles and roles configured in ~/.aws. It is read-only unless started
// with --edit, which allows deleting items.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/mikim83/dynamodb-client/internal/awsconfig"
	"github.com/mikim83/dynamodb-client/internal/ddb"
	"github.com/mikim83/dynamodb-client/internal/ui"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		profile     = flag.String("profile", os.Getenv("AWS_PROFILE"), "AWS profile from ~/.aws (default: $AWS_PROFILE, or pick one)")
		region      = flag.String("region", "", "AWS region (default: the profile's region)")
		endpoint    = flag.String("endpoint-url", "", "custom DynamoDB endpoint, e.g. http://localhost:8000 for DynamoDB Local")
		table       = flag.String("table", "", "open this table directly")
		pageSize    = flag.Int("page-size", 50, "rows per page")
		maxItems    = flag.Int("max-items", 10000, "maximum items read by \"load all\"")
		edit        = flag.Bool("edit", false, "allow deleting items (default: read-only)")
		showVersion = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()
	if *showVersion {
		fmt.Println("dynamodb-client", version)
		return nil
	}
	if *pageSize < 1 || *pageSize > 1000 {
		return fmt.Errorf("--page-size must be between 1 and 1000")
	}
	if *maxItems < 1 {
		return fmt.Errorf("--max-items must be positive")
	}

	current := *profile
	pick := current == ""
	for {
		if pick {
			name, ok, err := chooseProfile(current)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			current = name
		}

		cfg, err := awsconfig.Load(context.Background(), current, *region)
		if err != nil {
			return err
		}
		api := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
			if *endpoint != "" {
				o.BaseEndpoint = aws.String(*endpoint)
			}
		})

		var deleter ddb.Deleter
		if *edit {
			deleter = api
		}
		res, err := ui.Run(ui.Options{
			API:      api,
			Deleter:  deleter,
			Profile:  current,
			Region:   cfg.Region,
			PageSize: *pageSize,
			MaxItems: *maxItems,
			Table:    *table,
		})
		if err != nil {
			return err
		}
		if !res.SwitchProfile {
			return nil
		}
		pick, *table = true, ""
	}
}

// chooseProfile shows the picker when there is more than one profile. With
// none it returns "" so the SDK default chain (env vars, SSO, IMDS…) is used.
func chooseProfile(current string) (string, bool, error) {
	profiles, err := awsconfig.ListProfiles()
	if err != nil {
		return "", false, err
	}
	switch len(profiles) {
	case 0:
		return "", true, nil
	case 1:
		return profiles[0].Name, true, nil
	}
	return ui.PickProfile(profiles, current)
}
