package model

type Other struct {
	Limit       uint64   `json:"limit"`
	IDC         []string `json:"idc"`
	Query       bool     `json:"query"`
	Register    bool     `json:"register"`
	Export      bool     `json:"export"`
	ExQueryTime int      `json:"ex_query_time"`
	Domain      string   `json:"domain"`
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
	PushType bool   `json:"push_type"`
	Key      string `json:"key"`
}

type PermissionList struct {
	DDLSource   []string `json:"ddl_source"`
	DMLSource   []string `json:"dml_source"`
	QuerySource []string `json:"query_source"`
}
