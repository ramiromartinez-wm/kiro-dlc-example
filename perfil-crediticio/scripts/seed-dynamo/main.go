package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

const (
	defaultEndpoint = "http://dynamodb-local:8000"
)

// Actionable representa un item accionable para mejorar el perfil
type Actionable struct {
	Type            string `dynamodbav:"type"`
	Description     string `dynamodbav:"description"`
	ResolutionDays  int    `dynamodbav:"resolution_days"`
}

// UserCreditProfile representa el perfil crediticio de un usuario
type UserCreditProfile struct {
	UserID          string       `dynamodbav:"user_id"`
	DecisionState   string       `dynamodbav:"decision_state"`
	LineStatus      string       `dynamodbav:"line_status"`
	ProfileLevel    string       `dynamodbav:"profile_level"`
	BCRACutoffDate  string       `dynamodbav:"bcra_cutoff_date,omitempty"`
	PrimaryFactors  []string     `dynamodbav:"primary_factors"`
	Actionables     []Actionable `dynamodbav:"actionables"`
	TTL             int64        `dynamodbav:"ttl"`
}

// CreditDecisionHistory representa un registro histórico de decisión crediticia
type CreditDecisionHistory struct {
	UserID            string   `dynamodbav:"user_id"`
	DecisionTimestamp string   `dynamodbav:"decision_timestamp"`
	DecisionState     string   `dynamodbav:"decision_state"`
	LineStatus        string   `dynamodbav:"line_status"`
	ProfileLevel      string   `dynamodbav:"profile_level"`
	PrimaryFactors    []string `dynamodbav:"primary_factors"`
}

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

	// Seed user credit profiles
	if err := seedUserCreditProfiles(ctx, client); err != nil {
		slog.Error("Failed to seed user credit profiles", "error", err)
		os.Exit(1)
	}

	// Seed credit decisions history
	if err := seedCreditDecisionsHistory(ctx, client); err != nil {
		slog.Error("Failed to seed credit decisions history", "error", err)
		os.Exit(1)
	}

	slog.Info("Database seeded successfully")
}

func seedUserCreditProfiles(ctx context.Context, client *dynamodb.Client) error {
	slog.Info("Seeding user credit profiles")

	// TTL para 24 horas desde ahora
	ttl := time.Now().Add(24 * time.Hour).Unix()

	profiles := []UserCreditProfile{
		// user_001: Usuario aprobado
		{
			UserID:         "user_001",
			DecisionState:  "approved",
			LineStatus:     "active",
			ProfileLevel:   "bueno",
			BCRACutoffDate: "2024-10-01",
			PrimaryFactors: []string{
				"Historial crediticio estable y consistente",
				"Sin atrasos significativos en los últimos 12 meses",
				"Baja utilización de líneas de crédito existentes",
				"Antigüedad crediticia mayor a 2 años",
			},
			Actionables: []Actionable{
				{
					Type:           "directo",
					Description:    "Mantener los pagos al día para conservar el buen historial",
					ResolutionDays: 30,
				},
				{
					Type:           "directo",
					Description:    "Considerar aumentar el límite de crédito para mejorar el ratio de utilización",
					ResolutionDays: 60,
				},
				{
					Type:           "indirecto",
					Description:    "Diversificar tipos de crédito para fortalecer el perfil",
					ResolutionDays: 90,
				},
			},
			TTL: ttl,
		},
		// user_002: Usuario rechazado
		{
			UserID:         "user_002",
			DecisionState:  "rejected",
			LineStatus:     "rejected",
			ProfileLevel:   "crítico",
			BCRACutoffDate: "2024-09-15",
			PrimaryFactors: []string{
				"Situación 3 reportada en BCRA",
				"Morosidad mayor a 90 días en múltiples obligaciones",
				"Consultas crediticias excesivas en el último mes (más de 10)",
			},
			Actionables: []Actionable{
				{
					Type:           "indirecto",
					Description:    "Regularizar deudas en situación 3 para mejorar el perfil BCRA",
					ResolutionDays: 180,
				},
				{
					Type:           "indirecto",
					Description:    "Reducir consultas crediticias y esperar al menos 3 meses",
					ResolutionDays: 90,
				},
				{
					Type:           "indirecto",
					Description:    "Establecer plan de pagos para morosidades actuales",
					ResolutionDays: 120,
				},
			},
			TTL: ttl,
		},
		// user_003: Usuario bloqueado por política
		{
			UserID:        "user_003",
			DecisionState: "1P_rejected",
			LineStatus:    "blocked",
			ProfileLevel:  "bloqueado",
			PrimaryFactors: []string{
				"Bloqueado por política interna de la empresa",
			},
			Actionables: []Actionable{},
			TTL:         ttl,
		},
	}

	for _, profile := range profiles {
		item, err := attributevalue.MarshalMap(profile)
		if err != nil {
			return fmt.Errorf("failed to marshal profile for %s: %w", profile.UserID, err)
		}

		_, err = client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String("user_credit_profile"),
			Item:      item,
		})
		if err != nil {
			return fmt.Errorf("failed to put item for %s: %w", profile.UserID, err)
		}

		slog.Info("Inserted user credit profile", "user_id", profile.UserID, "decision_state", profile.DecisionState)
	}

	return nil
}

func seedCreditDecisionsHistory(ctx context.Context, client *dynamodb.Client) error {
	slog.Info("Seeding credit decisions history")

	now := time.Now()

	history := []CreditDecisionHistory{
		// Primer registro para user_001 (hace 30 días)
		{
			UserID:            "user_001",
			DecisionTimestamp: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
			DecisionState:     "approved",
			LineStatus:        "active",
			ProfileLevel:      "bueno",
			PrimaryFactors: []string{
				"Historial crediticio estable",
				"Sin atrasos recientes",
			},
		},
		// Segundo registro para user_001 (hace 60 días)
		{
			UserID:            "user_001",
			DecisionTimestamp: now.Add(-60 * 24 * time.Hour).Format(time.RFC3339),
			DecisionState:     "approved",
			LineStatus:        "active",
			ProfileLevel:      "muy bueno",
			PrimaryFactors: []string{
				"Excelente historial de pagos",
				"Baja utilización de crédito",
			},
		},
		// Registro para user_002
		{
			UserID:            "user_002",
			DecisionTimestamp: now.Add(-15 * 24 * time.Hour).Format(time.RFC3339),
			DecisionState:     "rejected",
			LineStatus:        "rejected",
			ProfileLevel:      "crítico",
			PrimaryFactors: []string{
				"Situación 3 en BCRA",
				"Morosidad mayor a 90 días",
				"Consultas crediticias excesivas",
			},
		},
	}

	for _, record := range history {
		item, err := attributevalue.MarshalMap(record)
		if err != nil {
			return fmt.Errorf("failed to marshal history for %s: %w", record.UserID, err)
		}

		_, err = client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String("credit_decisions_history"),
			Item:      item,
		})
		if err != nil {
			return fmt.Errorf("failed to put history item for %s: %w", record.UserID, err)
		}

		slog.Info("Inserted credit decision history", "user_id", record.UserID, "timestamp", record.DecisionTimestamp)
	}

	return nil
}
