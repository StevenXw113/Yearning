package model

type Other struct {
	Limit       uint64   `json:"limit"`
	IDC         []string `json:"idc"`
	Query       bool     `json:"query"`
	Register    bool     `json:"register"`
	Export      bool     `json:"export"`
	ExQueryTime int      `json:"ex_query_time"`
	Domain      string   `json:"domain"`
	// ManualExecute 为 true 时，末级审批通过只把工单置为「等待执行」(status=5)，
	// 由人工点「执行」触发；false（默认）保持审批通过即自动执行。
	ManualExecute bool `json:"manual_execute"`
}

type AI struct {
	BaseUrl          string  `json:"base_url"`
	APIKey           string  `json:"api_key"`
	FrequencyPenalty float32 `json:"frequency_penalty"`
	MaxTokens        int     `json:"max_tokens"`
	PresencePenalty  float32 `json:"presence_penalty"`
	Temperature      float32 `json:"temperature"`
	TopP             float32 `json:"top_p"`
	Model            string  `json:"model"`
	AdvisorPrompt    string  `json:"advisor_prompt"`
	SQLGenPrompt     string  `json:"sql_gen_prompt"`
	SQLAgentPrompt   string  `json:"sql_agent_prompt"`
	ProxyURL         string  `json:"proxy_url"`
	// MongoAdvisorPrompt / MongoSQLGenPrompt 是 MongoDB 数据源专用的提示词模板，
	// 留空时由后端使用内置默认（见 handler/fetch/ai.go 的 defaultMongo*Prompt）。
	MongoAdvisorPrompt string `json:"mongo_advisor_prompt"`
	MongoSQLGenPrompt  string `json:"mongo_sql_gen_prompt"`
	// Protocol 接入协议：openai（默认，OpenAI 兼容，含 Ollama / vLLM 等）、
	// deepseek（DeepSeek，同为 OpenAI 兼容）、anthropic（Claude Messages API）、
	// responses（OpenAI Responses API）
	Protocol string `json:"protocol"`
}

type Message struct {
	WebHook  string `json:"web_hook"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	ToUser   string `json:"to_user"`
	Mail     bool   `json:"mail"`
	Ding     bool   `json:"ding"`
	Ssl      bool   `json:"ssl"`
	Key      string `json:"key"`
}

type PermissionList struct {
	DDLSource   []string `json:"ddl_source"`
	DMLSource   []string `json:"dml_source"`
	QuerySource []string `json:"query_source"`
}
