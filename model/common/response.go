package common

import (
	"net/http"

	"gitee.com/taoJie_1/mall-agent/model/enum"
	"github.com/gin-gonic/gin"
)

type Response struct {
	Code  enum.ResCode `json:"code"`
	Data  interface{}  `json:"data"`
	Msg   enum.Msg     `json:"msg"`
	Token string       `json:"token,omitempty"`
}

type OllamaResponse struct {
	Response string `json:"response"`
}

func result(ctx *gin.Context, code enum.ResCode, msg enum.Msg, data interface{}) {
	ctx.JSON(http.StatusOK, Response{
		Code: code,
		Data: data,
		Msg:  msg,
	})
}

func resultWs(c chan *Response, code enum.ResCode, msg enum.Msg, data interface{}) {
	c <- &Response{
		Code: code,
		Data: data,
		Msg:  msg,
	}
}

// 带data
func Success(ctx *gin.Context, data interface{}) {
	result(ctx, enum.SuccessCode, enum.DefaultSuccessMsg, data)
}

func SuccessWs(c chan *Response, data interface{}) {
	resultWs(c, enum.SuccessCode, enum.DefaultSuccessMsg, data)
}

// 带msg,不带data
func SuccessOk(ctx *gin.Context, message string) {
	result(ctx, enum.SuccessCode, enum.Msg(message), map[string]interface{}{})
}

func SuccessAuth(ctx *gin.Context, token string, data interface{}) {
	ctx.JSON(http.StatusOK, Response{
		Code:  enum.SuccessCode,
		Data:  data,
		Msg:   enum.DefaultSuccessMsg,
		Token: token,
	})
}

func SuccessAuthWs(c chan *Response, token string) {
	c <- &Response{
		Code:  enum.SuccessCode,
		Data:  make(map[string]interface{}, 0),
		Msg:   enum.DefaultSuccessMsg,
		Token: token,
	}
}

func Fail(ctx *gin.Context, message string) {
	result(ctx, enum.ErrorCode, enum.Msg(message), map[string]interface{}{})
}

func FailWs(c chan *Response, message string) {
	resultWs(c, enum.ErrorCode, enum.Msg(message), map[string]interface{}{})
}

func FailNotFound(ctx *gin.Context) {
	ctx.JSON(http.StatusNotFound, Response{
		Code: enum.ErrorCode,
		Msg:  enum.DefaultFailMsg,
	})
}

// token过期
func FailAuth(ctx *gin.Context, message string) {
	ctx.AbortWithStatusJSON(http.StatusOK, Response{
		Code: enum.AuthErrorCode,
		Msg:  enum.Msg(message),
		Data: make(map[string]interface{}, 0),
	})
}

// token过期
func FailAuthWs(c chan *Response, message string) {
	resultWs(c, enum.AuthErrorCode, enum.Msg(message), map[string]interface{}{})
}
