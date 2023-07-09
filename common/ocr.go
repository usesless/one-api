package common

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

const API_KEY = "6r4HOZt9EKOPwfFTFpMwV6w0"
const SECRET_KEY = "RxNCtoWBWy0liikk1EjEma4XcfpXIpHm"

type Identifyresult struct {
	Results []BaiDuResult `json:"results"`
}

type BaiDuResult struct {
	Chars     []string `json:"chars"`
	WordsType string   `json:"words_type"`
	Words     Word     `json:"words"`
}

type Word struct {
	Word string `json:"word"`
}

func GetOcrBaiDu(imageRaw, urlRaw string) (string, error) {
	ocrBaiDuUrl := `https://aip.baidubce.com/rest/2.0/ocr/v1/doc_analysis?access_token=` + getAccessToken()
	str := ""
	if len(imageRaw) > 0 {
		str += `image=` + url.QueryEscape(imageRaw)
	}

	if len(urlRaw) > 0 {
		if len(str) > 0 {
			str += `&`
		}
		str += `url=` + url.QueryEscape(urlRaw)
	}
	payload := strings.NewReader(str)
	client := &http.Client{}
	fmt.Println(`请求百度OCR原始URL参数：`, ocrBaiDuUrl)
	fmt.Println(`请求百度OCR原始Body参数：`, str)
	req, err := http.NewRequest("POST", ocrBaiDuUrl, payload)
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	identifyStr := ""
	fmt.Println(`百度OCR结果返回原始数据：`, string(body))
	rawData := string(body)
	for {
		start := strings.Index(rawData, `"word":"`)
		end := strings.Index(rawData, `","line_probability"`)
		if start > 0 {
			identifyStr += rawData[start+8 : end]
			dataLen := len(rawData[start+8 : end])
			rawData = rawData[start+28+dataLen:]
		} else {
			break
		}
	}

	fmt.Println(`OCR结果特殊字符处理后返回数据：`, string(identifyStr))
	return identifyStr, nil
}

/**
 * 使用 AK，SK 生成鉴权签名（Access Token）
 * @return string 鉴权签名信息（Access Token）
 */
func getAccessToken() string {
	postData := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s", API_KEY, SECRET_KEY)
	resp, err := http.Post(`https://aip.baidubce.com/oauth/2.0/token`, "application/x-www-form-urlencoded", strings.NewReader(postData))
	if err != nil {
		return ""
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	accessTokenObj := map[string]string{}
	json.Unmarshal([]byte(body), &accessTokenObj)
	return accessTokenObj["access_token"]
}
