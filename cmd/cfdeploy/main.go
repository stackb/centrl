package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/stackb/centrl/pkg/cf"
)

func main() {
	log.SetPrefix("cfdeploy: ")
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	var (
		apiToken    = flag.String("api_token", "", "Cloudflare API token (or set CLOUDFLARE_API_TOKEN env var)")
		accountID   = flag.String("account_id", "", "Cloudflare account ID (or set CF_ACCOUNT_ID env var)")
		projectName = flag.String("project", "", "Cloudflare Pages project name")
		tarball     = flag.String("tarball", "", "Path to tarball containing deployment files")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Deploy a tarball to Cloudflare Pages without wrangler.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  CLOUDFLARE_API_TOKEN    Cloudflare API token (alternative to --api_token)\n")
		fmt.Fprintf(os.Stderr, "  CF_ACCOUNT_ID   Cloudflare account ID (alternative to --account_id)\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  export CLOUDFLARE_API_TOKEN=your-token-here\n")
		fmt.Fprintf(os.Stderr, "  export CF_ACCOUNT_ID=your-account-id-here\n")
		fmt.Fprintf(os.Stderr, "  %s --project=mysite --tarball=dist.tar\n", os.Args[0])
	}

	flag.Parse()

	// Get credentials from flags or environment
	token := *apiToken
	if token == "" {
		token = os.Getenv("CLOUDFLARE_API_TOKEN")
	}
	if token == "" {
		log.Fatal("API token required (use --api_token or CLOUDFLARE_API_TOKEN env var)")
	}

	acctID := *accountID
	if acctID == "" {
		acctID = os.Getenv("CF_ACCOUNT_ID")
	}
	if acctID == "" {
		log.Fatal("Account ID required (use --account_id or CF_ACCOUNT_ID env var)")
	}

	if *projectName == "" {
		log.Fatal("Project name required (use --project)")
	}

	if *tarball == "" {
		log.Fatal("Tarball path required (use --tarball)")
	}

	// Check if tarball exists
	if _, err := os.Stat(*tarball); os.IsNotExist(err) {
		log.Fatalf("Tarball not found: %s", *tarball)
	}

	// Create Cloudflare client
	client := cf.NewClient(token, acctID)

	// Check if project exists
	log.Printf("Checking project %s...", *projectName)
	project, err := client.GetProject(*projectName)
	if err != nil {
		log.Printf("Project not found, creating it...")
		project, err = client.CreateProject(*projectName, "main")
		if err != nil {
			log.Fatalf("Failed to create project: %v", err)
		}
		log.Printf("Created project: %s (subdomain: %s.pages.dev)", project.Name, project.Subdomain)
	} else {
		log.Printf("Found project: %s (subdomain: %s.pages.dev)", project.Name, project.Subdomain)
	}

	// Upload deployment
	log.Printf("Uploading deployment from %s...", *tarball)
	deployment, err := client.UploadDeployment(*projectName, *tarball)
	if err != nil {
		log.Fatalf("Failed to upload deployment: %v", err)
	}

	log.Printf("✓ Deployment successful!")
	log.Printf("  ID:          %s", deployment.ID)
	log.Printf("  URL:         %s", deployment.URL)
	log.Printf("  Environment: %s", deployment.Environment)
	log.Printf("  Stage:       %s", deployment.DeploymentStage)

	if len(deployment.Aliases) > 0 {
		log.Printf("  Aliases:")
		for _, alias := range deployment.Aliases {
			log.Printf("    - %s", alias)
		}
	}
}
