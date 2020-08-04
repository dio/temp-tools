package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	csdspb "github.com/envoyproxy/go-control-plane/envoy/service/status/v2"
)

// TestParseNodeMatcherWithFile tests parsing -request_file to nodematcher.
func TestParseNodeMatcherWithFile(t *testing.T) {
	c := Client{
		info: Flag{
			platform:    "gcp",
			requestFile: "./test_request.yaml",
		},
	}
	if err := c.parseNodeMatcher(); err != nil {
		t.Errorf("Parse NodeMatcher Error: %v", err)
	}
	if c.nodeMatcher == nil {
		t.Errorf("Parse NodeMatcher Failure!")
	}
	want := "{\"nodeId\":{\"exact\":\"fake_node_id\"},\"nodeMetadatas\":[{\"path\":[{\"key\":\"TRAFFICDIRECTOR_GCP_PROJECT_NUMBER\"}],\"value\":{\"stringMatch\":{\"exact\":\"fake_project_number\"}}},{\"path\":[{\"key\":\"TRAFFICDIRECTOR_NETWORK_NAME\"}],\"value\":{\"stringMatch\":{\"exact\":\"fake_network_name\"}}}]}"
	get, err := protojson.Marshal(c.nodeMatcher[0])
	if err != nil {
		t.Errorf("Parse NodeMatcher Error: %v", err)
	}

	if !shouldEqualJSON(t, string(get), want) {
		t.Errorf("NodeMatcher = \n%v\n, want: \n%v\n", string(get), want)
	}
}

// TestParseNodeMatcherWithString tests parsing -request_yaml to nodematcher.
func TestParseNodeMatcherWithString(t *testing.T) {
	c := Client{
		info: Flag{
			platform:    "gcp",
			requestYaml: "{\"node_matchers\": [{\"node_id\": {\"exact\": \"fake_node_id\"}, \"node_metadatas\": [{\"path\": [{\"key\": \"TRAFFICDIRECTOR_GCP_PROJECT_NUMBER\"}], \"value\": {\"string_match\": {\"exact\": \"fake_project_number\"}}}, {\"path\": [{\"key\": \"TRAFFICDIRECTOR_NETWORK_NAME\"}], \"value\": {\"string_match\": {\"exact\": \"fake_network_name\"}}}]}]}",
		},
	}
	err := c.parseNodeMatcher()
	if err != nil {
		t.Errorf("Parse NodeMatcher Error: %v", err)
	}
	if c.nodeMatcher == nil {
		t.Errorf("Parse NodeMatcher Failure!")
	}
	want := "{\"nodeId\":{\"exact\":\"fake_node_id\"}, \"nodeMetadatas\":[{\"path\":[{\"key\":\"TRAFFICDIRECTOR_GCP_PROJECT_NUMBER\"}], \"value\":{\"stringMatch\":{\"exact\":\"fake_project_number\"}}}, {\"path\":[{\"key\":\"TRAFFICDIRECTOR_NETWORK_NAME\"}], \"value\":{\"stringMatch\":{\"exact\":\"fake_network_name\"}}}]}"
	get, err := protojson.Marshal(c.nodeMatcher[0])
	if err != nil {
		t.Errorf("Parse NodeMatcher Error: %v", err)
	}
	if !shouldEqualJSON(t, string(get), want) {
		t.Errorf("NodeMatcher = \n%v\n, want: \n%v\n", string(get), want)
	}
}

// TestParseNodeMatcherWithFileAndString tests parsing -request_file and -request_yaml to nodematcher.
func TestParseNodeMatcherWithFileAndString(t *testing.T) {
	c := Client{
		info: Flag{
			platform:    "gcp",
			requestFile: "./test_request.yaml",
			requestYaml: "{\"node_matchers\": [{\"node_id\": {\"exact\": \"fake_node_id_from_cli\"}}]}",
		},
	}
	if err := c.parseNodeMatcher(); err != nil {
		t.Errorf("Parse NodeMatcher Error: %v", err)
	}
	if c.nodeMatcher == nil {
		t.Errorf("Parse NodeMatcher Failure!")
	}
	want := "{\"nodeId\":{\"exact\":\"fake_node_id_from_cli\"}, \"nodeMetadatas\":[{\"path\":[{\"key\":\"TRAFFICDIRECTOR_GCP_PROJECT_NUMBER\"}], \"value\":{\"stringMatch\":{\"exact\":\"fake_project_number\"}}}, {\"path\":[{\"key\":\"TRAFFICDIRECTOR_NETWORK_NAME\"}], \"value\":{\"stringMatch\":{\"exact\":\"fake_network_name\"}}}]}"
	get, err := protojson.Marshal(c.nodeMatcher[0])
	if err != nil {
		t.Errorf("Parse NodeMatcher Error: %v", err)
	}
	if !shouldEqualJSON(t, string(get), want) {
		t.Errorf("NodeMatcher = \n%v\n, want: \n%v\n", string(get), want)
	}
}

// captureOutput captures the stdout for testing.
func captureOutput(f func()) string {
	reader, writer, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	stdout := os.Stdout
	stderr := os.Stderr
	defer func() {
		os.Stdout = stdout
		os.Stderr = stderr
	}()
	os.Stdout = writer
	os.Stderr = writer
	out := make(chan string)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		var buf bytes.Buffer
		wg.Done()
		io.Copy(&buf, reader)
		out <- buf.String()
	}()
	wg.Wait()
	f()
	writer.Close()
	return <-out
}

// TestParseResponseWithoutNodeId tests post processing response without node_id.
func TestParseResponseWithoutNodeId(t *testing.T) {
	filename, _ := filepath.Abs("./response_without_nodeid_test.json")
	responsejson, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Errorf("Read From File Failure: %v", err)
	}
	var response csdspb.ClientStatusResponse
	if err = protojson.Unmarshal(responsejson, &response); err != nil {
		t.Errorf("Read From File Failure: %v", err)
	}
	out := captureOutput(func() {
		if err := printOutResponse(&response, ""); err != nil {
			t.Errorf("Print out response error: %v", err)
		}
	})
	want := "Client ID                                          xDS stream type                Config Status                  \ntest_node_1                                        test_stream_type1              N/A                            \ntest_node_2                                        test_stream_type2              N/A                            \ntest_node_3                                        test_stream_type3              N/A                            \n"
	if out != want {
		t.Errorf("want\n%vout\n%v", want, out)
	}
}

// TestParseResponseWithNodeId tests post processing response with node_id
func TestParseResponseWithNodeId(t *testing.T) {
	filename, _ := filepath.Abs("./response_with_nodeid_test.json")
	responsejson, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Errorf("Read From File Failure: %v", err)
	}
	var response csdspb.ClientStatusResponse
	if err = protojson.Unmarshal(responsejson, &response); err != nil {
		t.Errorf("Read From File Failure: %v", err)
	}
	out := captureOutput(func() {
		if err := printOutResponse(&response, "test_config.json"); err != nil {
			t.Errorf("Print out response error: %v", err)
		}
	})
	want := "Client ID                                          xDS stream type                Config Status                  \ntest_nodeid                                        test_stream_type1              RDS   STALE                    \n                                                                                  CDS   STALE                    \nConfig has been saved to test_config.json\n"
	if out != want {
		t.Errorf("want\n%vout\n%v", want, out)
	}

	outfile, _ := filepath.Abs("./test_config.json")
	outputjson, err := ioutil.ReadFile(outfile)
	if err != nil {
		t.Errorf("Write config to file failure: %v", err)
	}
	ok, err := equalJSONBytes(outputjson, responsejson)
	if err != nil {
		t.Errorf("failed to parse json")
	}
	if !ok {
		t.Errorf("Output formatted error")
	}
}

func shouldEqualJSON(t *testing.T, s1, s2 string) bool {
	t.Helper()

	verdict, err := equalJSONBytes([]byte(s1), []byte(s1))
	if err != nil {
		t.Errorf("failed to check since: %w", err)
		return false
	}

	return verdict
}

func equalJSONBytes(s1, s2 []byte) (bool, error) {
	var o1 interface{}
	var o2 interface{}

	var err error
	err = json.Unmarshal(s1, &o1)
	if err != nil {
		return false, fmt.Errorf("failed to marshal s1: %w", err)
	}
	err = json.Unmarshal(s2, &o2)
	if err != nil {
		return false, fmt.Errorf("failed to marshal s2: %w", err)
	}

	return reflect.DeepEqual(o1, o2), nil
}
