package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/ghodss/yaml"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_config_filter_http_router_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/router/v2"
	envoy_config_filter_network_http_connection_manager_v2 "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	csdspb "github.com/envoyproxy/go-control-plane/envoy/service/status/v2"
	envoy_type_matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
)

// isJson checks if str is a valid json format string
func isJson(str string) bool {
	input := []byte(str)
	decoder := json.NewDecoder(bytes.NewReader(input))
	for {
		_, err := decoder.Token()
		if err == io.EOF { // end of string
			break
		}
		if err != nil {
			return false
		}
	}
	return true
}

// parseYaml is a helper method for parsing csds request yaml to nodematchers
func parseYaml(path string, yamlStr string, nms *[]*envoy_type_matcher.NodeMatcher) error {
	if path != "" {
		// parse yaml to json
		filename, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		yamlFile, err := ioutil.ReadFile(filename)
		if err != nil {
			return err
		}
		js, err := yaml.YAMLToJSON(yamlFile)
		if err != nil {
			return err
		}

		// parse the json array to a map to iterate it
		var data map[string]interface{}
		if err = json.Unmarshal(js, &data); err != nil {
			return err
		}

		// parse each json object to proto
		for _, n := range data["node_matchers"].([]interface{}) {
			x := &envoy_type_matcher.NodeMatcher{}

			jsonString, err := json.Marshal(n)
			if err != nil {
				return err
			}
			if err = protojson.Unmarshal(jsonString, x); err != nil {
				return err
			}
			*nms = append(*nms, x)
		}
	}
	if yamlStr != "" {
		var js []byte
		var err error
		// json input
		if isJson(yamlStr) {
			js = []byte(yamlStr)
		} else {
			// parse the yaml input into json
			js, err = yaml.YAMLToJSON([]byte(yamlStr))
			if err != nil {
				return err
			}
		}

		// parse the json array to a map to iterate it
		var data map[string]interface{}
		if err = json.Unmarshal(js, &data); err != nil {
			return err
		}

		// parse each json object to proto
		for i, n := range data["node_matchers"].([]interface{}) {
			x := &envoy_type_matcher.NodeMatcher{}

			jsonString, err := json.Marshal(n)
			if err != nil {
				return err
			}
			if err = protojson.Unmarshal(jsonString, x); err != nil {
				return err
			}

			// merge the proto with existing proto from request_file
			if i < len(*nms) {
				proto.Merge((*nms)[i], x)
			} else {
				*nms = append(*nms, x)
			}
		}
	}
	return nil
}

// getValueByKeyFromNodeMatcher gets the first value by key from the metadata of a set of NodeMatchers
func getValueByKeyFromNodeMatcher(nms []*envoy_type_matcher.NodeMatcher, key string) string {
	for _, nm := range nms {
		for _, mt := range nm.NodeMetadatas {
			for _, path := range mt.Path {
				if path.GetKey() == key {
					return mt.Value.GetStringMatch().GetExact()
				}
			}
		}
	}
	return ""
}

// TypeResolver implements protoregistry.ExtensionTypeResolver and protoregistry.MessageTypeResolver to resolve google.protobuf.Any types
type TypeResolver struct{}

func (r *TypeResolver) FindMessageByName(message protoreflect.FullName) (protoreflect.MessageType, error) {
	return nil, protoregistry.NotFound
}

// FindMessageByURL links the message type url to the specific message type
// TODO: If there's other message type can be passed in google.protobuf.Any, the typeUrl and
//  messageType need to be added to this method to make sure it can be parsed and output correctly
func (r *TypeResolver) FindMessageByURL(url string) (protoreflect.MessageType, error) {
	switch url {
	case "type.googleapis.com/envoy.config.filter.network.http_connection_manager.v2.HttpConnectionManager":
		httpConnectionManager := envoy_config_filter_network_http_connection_manager_v2.HttpConnectionManager{}
		return httpConnectionManager.ProtoReflect().Type(), nil
	case "type.googleapis.com/envoy.api.v2.Cluster":
		cluster := envoy_api_v2.Cluster{}
		return cluster.ProtoReflect().Type(), nil
	case "type.googleapis.com/envoy.api.v2.Listener":
		listener := envoy_api_v2.Listener{}
		return listener.ProtoReflect().Type(), nil
	case "type.googleapis.com/envoy.config.filter.http.router.v2.Router":
		router := envoy_config_filter_http_router_v2.Router{}
		return router.ProtoReflect().Type(), nil
	case "type.googleapis.com/envoy.api.v2.RouteConfiguration":
		routeConfiguration := envoy_api_v2.RouteConfiguration{}
		return routeConfiguration.ProtoReflect().Type(), nil
	default:
		return nil, protoregistry.NotFound
	}
}

func (r *TypeResolver) FindExtensionByName(field protoreflect.FullName) (protoreflect.ExtensionType, error) {
	return nil, protoregistry.NotFound
}

func (r *TypeResolver) FindExtensionByNumber(message protoreflect.FullName, field protoreflect.FieldNumber) (protoreflect.ExtensionType, error) {
	return nil, protoregistry.NotFound
}

// parseConfigStatus parses each xds config status to string
func parseConfigStatus(xdsConfig []*csdspb.PerXdsConfig) []string {
	var configStatus []string
	for _, perXdsConfig := range xdsConfig {
		status := perXdsConfig.GetStatus().String()
		var xds string
		if perXdsConfig.GetClusterConfig() != nil {
			xds = "CDS"
		} else if perXdsConfig.GetListenerConfig() != nil {
			xds = "LDS"
		} else if perXdsConfig.GetRouteConfig() != nil {
			xds = "RDS"
		} else if perXdsConfig.GetScopedRouteConfig() != nil {
			xds = "SRDS"
		}
		if status != "" && xds != "" {
			configStatus = append(configStatus, xds+"   "+status)
		}
	}
	return configStatus
}

// printOutResponse processes response and print
func printOutResponse(response *csdspb.ClientStatusResponse, fileName string) error {
	if response.GetConfig() == nil || len(response.GetConfig()) == 0 {
		fmt.Printf("No xDS clients connected.\n")
		return nil
	} else {
		fmt.Printf("%-50s %-30s %-30s \n", "Client ID", "xDS stream type", "Config Status")
	}

	var hasXdsConfig bool

	for _, config := range response.GetConfig() {
		var id string
		var xdsType string
		if config.GetNode() != nil {
			id = config.GetNode().GetId()
			metadata := config.GetNode().GetMetadata().AsMap()

			// control plane is expected to use "XDS_STREAM_TYPE" to communicate
			// the stream type of the connected client in the response.
			if metadata["XDS_STREAM_TYPE"] != nil {
				xdsType = metadata["XDS_STREAM_TYPE"].(string)
			}

		}

		if config.GetXdsConfig() == nil {
			if config.GetNode() != nil {
				fmt.Printf("%-50s %-30s %-30s \n", id, xdsType, "N/A")
			}
		} else {
			hasXdsConfig = true

			// parse config status
			configStatus := parseConfigStatus(config.GetXdsConfig())
			fmt.Printf("%-50s %-30s ", id, xdsType)

			for i := 0; i < len(configStatus); i++ {
				if i == 0 {
					fmt.Printf("%-30s \n", configStatus[i])
				} else {
					fmt.Printf("%-50s %-30s %-30s \n", "", "", configStatus[i])
				}
			}
			if len(configStatus) == 0 {
				fmt.Printf("\n")
			}
		}
	}

	if hasXdsConfig {
		// parse response to json
		// format the json and resolve google.protobuf.Any types
		m := protojson.MarshalOptions{Multiline: true, Indent: "  ", Resolver: &TypeResolver{}}
		out, err := m.Marshal(response)
		if err != nil {
			return err
		}

		if fileName == "" {
			// output the configuration to stdout by default
			fmt.Println("Detailed Config:")
			fmt.Println(string(out))
		} else {
			// write the configuration to the file
			f, err := os.Create(fileName)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = f.Write(out)
			if err != nil {
				return err
			}
			fmt.Printf("Config has been saved to %v\n", fileName)
		}
	}
	return nil
}
