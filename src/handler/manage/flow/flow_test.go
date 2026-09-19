//go:build dbtest

// 这套用例是上游遗留的集成测试：需要可用的元数据库、并会启动完整 HTTP 栈，
// 因此用 dbtest 构建标签隔离，默认不参与 go test ./...（CI 里也就不会因缺库而红）。
//
// 需要时执行：go test -tags dbtest ./src/handler/manage/flow/
package flow

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/model"
	"Yearning-go/src/test"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/http"
	"os"
	"testing"
)

func setup() {
	//model.NewDBSub()
	apis.NewTest()
}

func teardown() {
	model.DB().Model(model.CoreWorkflowTpl{}).Where("source =?", "test").Delete(&model.CoreWorkflowTpl{})
}

var apis = test.Case{
	Method:  http.MethodPost,
	Uri:     "/api/v2/manage/flow",
	Handler: TplRestApis(),
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}

func TestGeneralAllSources(t *testing.T) {
	var Ref common.Resp
	apis.Get("").Do().Unmarshal(&Ref)
	assert.NotEqual(t, 0, len(Ref.Payload.([]interface{})))
	assert.Equal(t, 1200, Ref.Code)
}

func TestTplPostSourceTemplate(t *testing.T) {
	var Ref common.Resp
	args := `{"steps":[{"desc": "提交阶段", "type": 0, "auditor": ["提交人"]}, {"desc": "321", "type": 1, "auditor": ["admin", "hj"]}],"source":"test"}`
	apis.Post(args).Do().Unmarshal(&Ref)
	assert.Equal(t, i18n.DefaultLang.Load(i18n.INFO_DATA_IS_UPDATED), Ref.Text)

	args = `{"steps":[{"desc": "提交阶段", "type": 0, "auditor": ["提交人"]}, {"desc": "321", "type": 2, "auditor": ["admin", "hj"]}],"source":"test"}`
	apis.Post(args).Do().Unmarshal(&Ref)
	var tpl model.CoreWorkflowTpl
	var steps []Tpl
	model.DB().Where("source =?", "test").Find(&tpl)
	_ = json.Unmarshal(tpl.Steps, &steps)
	assert.Equal(t, 2, steps[1].Type)

	args = `{"steps:{"desc": "提交阶段", "type": 0, "auditor": ["提交人"]}, {"desc": "321", "type": 2, "auditor": ["admin", "hj"]}],"source":"test"}`
	apis.Post(args).Do().Unmarshal(&Ref)
	assert.Equal(t, 1310, Ref.Code)
}

func TestTplEditSourceTemplateInfo(t *testing.T) {
	var Ref common.Resp
	apis.Put(`{"source": "test"}`).Do().Unmarshal(&Ref)
	assert.NotNil(t, Ref.Payload.(map[string]interface{})["steps"])
	apis.Put(`{"source": 123}`).Do().Unmarshal(&Ref)
	assert.Equal(t, 1310, Ref.Code)
}

func TestGeneralAllSources2(t *testing.T) {
	fmt.Println(1<<20 - 1)
}
