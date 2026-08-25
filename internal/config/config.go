package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env      string
	App      AppConfig
	DB       DBConfig
	Redis    RedisConfig
	Deepgram DeepgramConfig
	Twilio   TwilioConfig
	Supabase SupabaseConfig
	AWS      AWSConfig
}

type AppConfig struct {
	Port            string
	APIBaseURL      string
	FrontendURL     string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

type DBConfig struct {
	DSN             string
	MinConns        int32
	MaxConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	ConnectTimeout  time.Duration
}

type RedisConfig struct {
	URL      string
	Password string
	DB       int
}

type DeepgramConfig struct {
	APIKey         string
	AgentURL       string
	ListenModel    string
	ListenLanguage string
	LanguageHints  []string
	SpeakProvider  string
	SpeakModel     string
	SpeakModelID   string
	SpeakVoice     string
	SpeakLanguage  string
	SpeakVersion   string
	SpeakSpeed     float64
	ThinkProvider  string
	ThinkModel     string
	BaseURL        string
	OpenAIAPIKey   string
}

type TwilioConfig struct {
	AccountSID                string
	AuthToken                 string
	VoiceNumber               string
	WhatsAppNumber            string
	StatusCallbackURL         string
	WhatsAppStatusCallbackURL string
	MediaStreamURL            string
	TrialMode                 bool
	RecordingEnabled          bool
	TrialVoiceURL             string
}

type SupabaseConfig struct {
	URL            string
	ServiceRoleKey string
	AnonKey        string
	StorageBucket  string
}

type AWSConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	S3Bucket        string
	S3PublicBaseURL string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	env := strings.ToLower(
		strings.TrimSpace(
			getEnv(
				"APP_ENV",
				"development",
			),
		),
	)

	cfg := &Config{
		Env: env,

		App: AppConfig{
			Port: getEnv(
				"PORT",
				"8080",
			),

			APIBaseURL: strings.TrimRight(
				getEnv(
					"API_BASE_URL",
					"http://localhost:8080",
				),
				"/",
			),

			FrontendURL: strings.TrimRight(
				getEnv(
					"FRONTEND_URL",
					"http://localhost:5173",
				),
				"/",
			),

			ReadTimeout: getEnvDuration(
				"READ_TIMEOUT",
				15*time.Second,
			),

			WriteTimeout: getEnvDuration(
				"WRITE_TIMEOUT",
				15*time.Second,
			),

			IdleTimeout: getEnvDuration(
				"IDLE_TIMEOUT",
				60*time.Second,
			),

			ShutdownTimeout: getEnvDuration(
				"SHUTDOWN_TIMEOUT",
				10*time.Second,
			),
		},

		DB: DBConfig{
			DSN: getEnv(
				"DATABASE_URL",
				"",
			),

			MinConns: int32(
				getEnvInt(
					"DB_MIN_CONNS",
					2,
				),
			),

			MaxConns: int32(
				getEnvInt(
					"DB_MAX_CONNS",
					10,
				),
			),

			MaxConnLifetime: getEnvDuration(
				"DB_MAX_CONN_LIFETIME",
				time.Hour,
			),

			MaxConnIdleTime: getEnvDuration(
				"DB_MAX_CONN_IDLE_TIME",
				30*time.Minute,
			),

			ConnectTimeout: getEnvDuration(
				"DB_CONNECT_TIMEOUT",
				10*time.Second,
			),
		},

		Redis: RedisConfig{
			URL: getEnv(
				"REDIS_URL",
				"redis://localhost:6379/0",
			),

			Password: getEnv(
				"REDIS_PASSWORD",
				"",
			),

			DB: getEnvInt(
				"REDIS_DB",
				0,
			),
		},

		Deepgram: DeepgramConfig{
			APIKey: getEnv(
				"DEEPGRAM_API_KEY",
				"",
			),

			AgentURL: getEnv(
				"DEEPGRAM_AGENT_URL",
				"wss://agent.deepgram.com/v1/agent/converse",
			),

			ListenModel: getEnv(
				"DEEPGRAM_LISTEN_MODEL",
				"nova-3",
			),

			ListenLanguage: getEnv(
				"DEEPGRAM_LISTEN_LANGUAGE",
				"multi",
			),

			LanguageHints: getEnvList(
				"DEEPGRAM_LANGUAGE_HINTS",
				[]string{
					"en",
					"hi",
					"te",
				},
			),

			SpeakProvider: getEnv(
				"DEEPGRAM_SPEAK_PROVIDER",
				"open_ai",
			),

			SpeakModel: getEnv(
				"DEEPGRAM_SPEAK_MODEL",
				"gpt-4o-mini-tts",
			),

			SpeakModelID: getEnv(
				"DEEPGRAM_SPEAK_MODEL_ID",
				"",
			),

			SpeakVoice: getEnv(
				"DEEPGRAM_SPEAK_VOICE",
				"alloy",
			),

			SpeakLanguage: getEnv(
				"DEEPGRAM_SPEAK_LANGUAGE",
				"multi",
			),

			SpeakVersion: getEnv(
				"DEEPGRAM_SPEAK_VERSION",
				"v1",
			),

			SpeakSpeed: getEnvFloat(
				"DEEPGRAM_SPEAK_SPEED",
				1.0,
			),

			ThinkProvider: getEnv(
				"DEEPGRAM_THINK_PROVIDER",
				"open_ai",
			),

			ThinkModel: getEnv(
				"DEEPGRAM_THINK_MODEL",
				"gpt-5.6-luna",
			),

			BaseURL: strings.TrimRight(
				getEnv(
					"DEEPGRAM_BASE_URL",
					"https://api.deepgram.com",
				),
				"/",
			),

			OpenAIAPIKey: getEnv(
				"OPENAI_API_KEY",
				"",
			),
		},

		Twilio: TwilioConfig{
			AccountSID: getEnv(
				"TWILIO_ACCOUNT_SID",
				"",
			),

			AuthToken: getEnv(
				"TWILIO_AUTH_TOKEN",
				"",
			),

			VoiceNumber: getEnv(
				"TWILIO_VOICE_NUMBER",
				"",
			),

			WhatsAppNumber: getEnv(
				"TWILIO_WHATSAPP_NUMBER",
				"",
			),

			StatusCallbackURL: getEnv(
				"TWILIO_STATUS_CALLBACK_URL",
				"",
			),

			WhatsAppStatusCallbackURL: getEnv(
				"TWILIO_WHATSAPP_STATUS_CALLBACK_URL",
				"",
			),

			MediaStreamURL: strings.TrimRight(
				getEnv(
					"TWILIO_MEDIA_STREAM_URL",
					"",
				),
				"/",
			),

			TrialMode: getEnvBool(
				"TWILIO_TRIAL_MODE",
				false,
			),

			RecordingEnabled: getEnvBool(
				"TWILIO_RECORDING_ENABLED",
				true,
			),

			TrialVoiceURL: getEnv(
				"TWILIO_TRIAL_VOICE_URL",
				"https://webhooks.twilio.com/v1/Voice/Template/voice_speech_recognition",
			),
		},

		Supabase: SupabaseConfig{
			URL: strings.TrimRight(
				getEnv(
					"SUPABASE_URL",
					"",
				),
				"/",
			),

			ServiceRoleKey: getEnv(
				"SUPABASE_SERVICE_ROLE_KEY",
				"",
			),

			AnonKey: getEnv(
				"SUPABASE_ANON_KEY",
				"",
			),

			StorageBucket: getEnv(
				"SUPABASE_STORAGE_BUCKET",
				"assets",
			),
		},

		AWS: AWSConfig{
			Region: getEnv(
				"AWS_REGION",
				"ap-south-1",
			),

			AccessKeyID: getEnv(
				"AWS_ACCESS_KEY_ID",
				"",
			),

			SecretAccessKey: getEnv(
				"AWS_SECRET_ACCESS_KEY",
				"",
			),

			S3Bucket: getEnv(
				"AWS_S3_BUCKET",
				"",
			),

			S3PublicBaseURL: strings.TrimRight(
				getEnv(
					"AWS_S3_PUBLIC_BASE_URL",
					"",
				),
				"/",
			),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	var missing []string

	if strings.TrimSpace(c.DB.DSN) == "" {
		missing = append(
			missing,
			"DATABASE_URL",
		)
	}

	if strings.TrimSpace(c.App.APIBaseURL) == "" {
		missing = append(
			missing,
			"API_BASE_URL",
		)
	}

	if c.App.Port == "" {
		missing = append(
			missing,
			"PORT",
		)
	}

	if c.DB.MinConns < 0 {
		return fmt.Errorf(
			"DB_MIN_CONNS cannot be negative",
		)
	}

	if c.DB.MaxConns <= 0 {
		return fmt.Errorf(
			"DB_MAX_CONNS must be greater than zero",
		)
	}

	if c.DB.MinConns > c.DB.MaxConns {
		return fmt.Errorf(
			"DB_MIN_CONNS cannot exceed DB_MAX_CONNS",
		)
	}

	if c.Deepgram.SpeakSpeed <= 0 {
		return fmt.Errorf(
			"DEEPGRAM_SPEAK_SPEED must be greater than zero",
		)
	}

	if strings.EqualFold(
		c.Deepgram.SpeakProvider,
		"open_ai",
	) && strings.TrimSpace(
		c.Deepgram.OpenAIAPIKey,
	) == "" {
		return fmt.Errorf(
			"OPENAI_API_KEY cannot be empty when DEEPGRAM_SPEAK_PROVIDER=open_ai",
		)
	}

	if c.Twilio.TrialMode &&
		strings.TrimSpace(
			c.Twilio.TrialVoiceURL,
		) == "" {
		return fmt.Errorf(
			"TWILIO_TRIAL_VOICE_URL cannot be empty when TWILIO_TRIAL_MODE=true",
		)
	}

	if c.Env == "production" {
		var missingProd []string

		if c.Deepgram.APIKey == "" {
			missingProd = append(
				missingProd,
				"DEEPGRAM_API_KEY",
			)
		}

		if c.Deepgram.AgentURL == "" {
			missingProd = append(
				missingProd,
				"DEEPGRAM_AGENT_URL",
			)
		}

		if c.Twilio.AccountSID == "" {
			missingProd = append(
				missingProd,
				"TWILIO_ACCOUNT_SID",
			)
		}

		if c.Twilio.AuthToken == "" {
			missingProd = append(
				missingProd,
				"TWILIO_AUTH_TOKEN",
			)
		}

		if c.Twilio.VoiceNumber == "" {
			missingProd = append(
				missingProd,
				"TWILIO_VOICE_NUMBER",
			)
		}

		if c.Twilio.TrialMode {
			if c.Twilio.TrialVoiceURL == "" {
				missingProd = append(
					missingProd,
					"TWILIO_TRIAL_VOICE_URL",
				)
			}
		} else if c.Twilio.MediaStreamURL == "" {
			missingProd = append(
				missingProd,
				"TWILIO_MEDIA_STREAM_URL",
			)
		}

		if c.AWS.S3Bucket == "" {
			missingProd = append(
				missingProd,
				"AWS_S3_BUCKET",
			)
		}

		if len(missingProd) > 0 {
			return fmt.Errorf(
				"missing required production environment variables: %s",
				strings.Join(
					missingProd,
					", ",
				),
			)
		}
	}

	return validateURL(
		c.App.APIBaseURL,
		"API_BASE_URL",
	)
}

func validateURL(
	value string,
	name string,
) error {
	parsed, err := url.Parse(
		strings.TrimSpace(value),
	)
	if err != nil {
		return fmt.Errorf(
			"%s is invalid: %w",
			name,
			err,
		)
	}

	if parsed.Scheme == "" ||
		parsed.Host == "" {
		return fmt.Errorf(
			"%s must contain scheme and host",
			name,
		)
	}

	return nil
}

func (c *Config) Integrations() map[string]bool {
	return map[string]bool{
		"aws_s3": strings.TrimSpace(
			c.AWS.S3Bucket,
		) != "",

		"deepgram": strings.TrimSpace(
			c.Deepgram.APIKey,
		) != "",

		"openai": strings.TrimSpace(
			c.Deepgram.OpenAIAPIKey,
		) != "",

		"supabase": strings.TrimSpace(
			c.Supabase.URL,
		) != "" &&
			strings.TrimSpace(
				c.Supabase.ServiceRoleKey,
			) != "",

		"twilio": strings.TrimSpace(
			c.Twilio.AccountSID,
		) != "" &&
			strings.TrimSpace(
				c.Twilio.AuthToken,
			) != "",

		"whatsapp": strings.TrimSpace(
			c.Twilio.WhatsAppNumber,
		) != "",
	}
}

func getEnv(
	key string,
	fallback string,
) string {
	value, ok := os.LookupEnv(
		key,
	)

	if !ok {
		return fallback
	}

	value = strings.TrimSpace(
		value,
	)

	if value == "" {
		return fallback
	}

	return value
}

func getEnvList(
	key string,
	fallback []string,
) []string {
	value, ok := os.LookupEnv(
		key,
	)

	if !ok ||
		strings.TrimSpace(value) == "" {
		return fallback
	}

	parts := strings.Split(
		value,
		",",
	)

	result := make(
		[]string,
		0,
		len(parts),
	)

	for _, part := range parts {
		part = strings.TrimSpace(
			part,
		)

		if part != "" {
			result = append(
				result,
				part,
			)
		}
	}

	if len(result) == 0 {
		return fallback
	}

	return result
}

func getEnvInt(
	key string,
	fallback int,
) int {
	value, ok := os.LookupEnv(
		key,
	)

	if !ok ||
		strings.TrimSpace(value) == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(
		strings.TrimSpace(value),
	)
	if err != nil {
		return fallback
	}

	return parsed
}

func getEnvFloat(
	key string,
	fallback float64,
) float64 {
	value, ok := os.LookupEnv(
		key,
	)

	if !ok ||
		strings.TrimSpace(value) == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(
		strings.TrimSpace(value),
		64,
	)
	if err != nil {
		return fallback
	}

	return parsed
}

func getEnvDuration(
	key string,
	fallback time.Duration,
) time.Duration {
	value, ok := os.LookupEnv(
		key,
	)

	if !ok ||
		strings.TrimSpace(value) == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(
		strings.TrimSpace(value),
	)
	if err != nil {
		return fallback
	}

	return parsed
}

func getEnvBool(
	key string,
	fallback bool,
) bool {
	value, ok := os.LookupEnv(
		key,
	)

	if !ok ||
		strings.TrimSpace(value) == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(
		strings.TrimSpace(value),
	)
	if err != nil {
		return fallback
	}

	return parsed
}
