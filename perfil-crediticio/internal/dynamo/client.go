package dynamo

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Client es un cliente DynamoDB con operaciones CRUD
type Client struct {
	client *dynamodb.Client
}

// UserProfile representa el perfil de usuario almacenado en DynamoDB
type UserProfile struct {
	UserID         string                 `dynamodbav:"userID" json:"userId"`
	Name           string                 `dynamodbav:"name" json:"name"`
	Email          string                 `dynamodbav:"email" json:"email"`
	CreditScore    int                    `dynamodbav:"creditScore" json:"creditScore"`
	RiskLevel      string                 `dynamodbav:"riskLevel" json:"riskLevel"`
	BCRAData       map[string]interface{} `dynamodbav:"bcraData" json:"bcraData"`
	LastUpdated    string                 `dynamodbav:"lastUpdated" json:"lastUpdated"`
	OnboardingDate string                 `dynamodbav:"onboardingDate,omitempty" json:"onboardingDate,omitempty"`
}

// Decision representa una decisión de crédito histórica
type Decision struct {
	DecisionID  string    `dynamodbav:"decisionID" json:"decisionId"`
	UserID      string    `dynamodbav:"userID" json:"userId"`
	Timestamp   string    `dynamodbav:"timestamp" json:"timestamp"`
	Decision    string    `dynamodbav:"decision" json:"decision"`
	Amount      float64   `dynamodbav:"amount" json:"amount"`
	Reason      string    `dynamodbav:"reason" json:"reason"`
	CreditScore int       `dynamodbav:"creditScore" json:"creditScore"`
	TTL         int64     `dynamodbav:"ttl,omitempty" json:"ttl,omitempty"`
}

// DisputeLog representa un registro de disputa
type DisputeLog struct {
	DisputeID   string    `dynamodbav:"disputeID" json:"disputeId"`
	UserID      string    `dynamodbav:"userID" json:"userId"`
	Timestamp   string    `dynamodbav:"timestamp" json:"timestamp"`
	IssueType   string    `dynamodbav:"issueType" json:"issueType"`
	Description string    `dynamodbav:"description" json:"description"`
	Status      string    `dynamodbav:"status" json:"status"`
	CreatedAt   time.Time `dynamodbav:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time `dynamodbav:"updatedAt" json:"updatedAt"`
}

// NewClient crea un nuevo cliente DynamoDB
func NewClient(endpoint string) (*Client, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
	)
	if err != nil {
		return nil, fmt.Errorf("dynamo NewClient: failed to load config: %w", err)
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	slog.Info("dynamo client initialized", "endpoint", endpoint)
	return &Client{client: client}, nil
}

// GetUserProfile obtiene el perfil de usuario por userID
func (c *Client) GetUserProfile(ctx context.Context, userID string) (*UserProfile, error) {
	result, err := c.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String("UserProfiles"),
		Key: map[string]types.AttributeValue{
			"userID": &types.AttributeValueMemberS{Value: userID},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("dynamo GetUserProfile: failed to get item: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("dynamo GetUserProfile: user not found: %s", userID)
	}

	var profile UserProfile
	err = attributevalue.UnmarshalMap(result.Item, &profile)
	if err != nil {
		return nil, fmt.Errorf("dynamo GetUserProfile: failed to unmarshal: %w", err)
	}

	return &profile, nil
}

// PutUserProfile guarda o actualiza un perfil de usuario
func (c *Client) PutUserProfile(ctx context.Context, profile *UserProfile) error {
	item, err := attributevalue.MarshalMap(profile)
	if err != nil {
		return fmt.Errorf("dynamo PutUserProfile: failed to marshal: %w", err)
	}

	_, err = c.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String("UserProfiles"),
		Item:      item,
	})

	if err != nil {
		return fmt.Errorf("dynamo PutUserProfile: failed to put item: %w", err)
	}

	slog.InfoContext(ctx, "user profile saved", "userID", profile.UserID)
	return nil
}

// GetDecisionsHistory obtiene el historial de decisiones de un usuario
func (c *Client) GetDecisionsHistory(ctx context.Context, userID string) ([]Decision, error) {
	result, err := c.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String("DecisionsHistory"),
		KeyConditionExpression: aws.String("userID = :userID"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":userID": &types.AttributeValueMemberS{Value: userID},
		},
		ScanIndexForward: aws.Bool(false), // Orden descendente por timestamp
		Limit:            aws.Int32(10),   // Últimas 10 decisiones
	})

	if err != nil {
		return nil, fmt.Errorf("dynamo GetDecisionsHistory: failed to query: %w", err)
	}

	var decisions []Decision
	err = attributevalue.UnmarshalListOfMaps(result.Items, &decisions)
	if err != nil {
		return nil, fmt.Errorf("dynamo GetDecisionsHistory: failed to unmarshal: %w", err)
	}

	return decisions, nil
}

// LogDispute registra una nueva disputa
func (c *Client) LogDispute(ctx context.Context, entry *DisputeLog) error {
	// Asegurar timestamps
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	entry.UpdatedAt = time.Now()

	item, err := attributevalue.MarshalMap(entry)
	if err != nil {
		return fmt.Errorf("dynamo LogDispute: failed to marshal: %w", err)
	}

	_, err = c.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String("DisputesLog"),
		Item:      item,
	})

	if err != nil {
		return fmt.Errorf("dynamo LogDispute: failed to put item: %w", err)
	}

	slog.InfoContext(ctx, "dispute logged",
		"disputeID", entry.DisputeID,
		"userID", entry.UserID,
		"issueType", entry.IssueType,
	)
	return nil
}
