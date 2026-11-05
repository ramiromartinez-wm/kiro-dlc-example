package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	defaultEndpoint = "http://dynamodb-local:8000"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx := context.Background()

	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	slog.Info("Initializing DynamoDB client", "endpoint", endpoint)

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
	)
	if err != nil {
		slog.Error("Failed to load AWS config", "error", err)
		os.Exit(1)
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	tables := []struct {
		name       string
		createFunc func(context.Context, *dynamodb.Client) error
	}{
		{
			name:       "user_credit_profile",
			createFunc: createUserCreditProfileTable,
		},
		{
			name:       "credit_decisions_history",
			createFunc: createCreditDecisionsHistoryTable,
		},
		{
			name:       "dispute_audit_log",
			createFunc: createDisputeAuditLogTable,
		},
	}

	for _, table := range tables {
		slog.Info("Creating table", "table", table.name)
		
		if err := table.createFunc(ctx, client); err != nil {
			var resourceInUse *types.ResourceInUseException
			if errors.As(err, &resourceInUse) {
				slog.Info("Table already exists, skipping", "table", table.name)
				continue
			}
			
			slog.Error("Failed to create table", "table", table.name, "error", err)
			os.Exit(1)
		}
		
		slog.Info("Table created successfully", "table", table.name)
	}

	slog.Info("All tables initialized successfully")
}

func createUserCreditProfileTable(ctx context.Context, client *dynamodb.Client) error {
	// Create the table
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String("user_credit_profile"),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("user_id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("user_id"),
				KeyType:       types.KeyTypeHash,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		return err
	}

	// Enable TTL on the table
	_, err = client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String("user_credit_profile"),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			Enabled:       aws.Bool(true),
			AttributeName: aws.String("ttl"),
		},
	})
	return err
}

func createCreditDecisionsHistoryTable(ctx context.Context, client *dynamodb.Client) error {
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String("credit_decisions_history"),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("user_id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("decision_timestamp"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("user_id"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("decision_timestamp"),
				KeyType:       types.KeyTypeRange,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	return err
}

func createDisputeAuditLogTable(ctx context.Context, client *dynamodb.Client) error {
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String("dispute_audit_log"),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("user_id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("timestamp"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("user_id"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("timestamp"),
				KeyType:       types.KeyTypeRange,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	return err
}
