// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package envoyfilter

import (
	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/golang/protobuf/proto"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pilot/pkg/networking/util"
	"istio.io/istio/pilot/pkg/util/runtime"
	"istio.io/istio/pkg/config/host"
	"istio.io/pkg/log"
)

func ApplyClusterMerge(pctx networking.EnvoyFilter_PatchContext, efw *model.EnvoyFilterWrapper, c *cluster.Cluster) (out *cluster.Cluster) {
	defer runtime.HandleCrash(runtime.LogPanic, func(interface{}) {
		log.Errorf("clusters patch caused panic, so the patches did not take effect")
	})
	// In case the patches cause panic, use the clusters generated before to reduce the influence.
	out = c
	if efw == nil {
		return
	}
	for _, cp := range efw.Patches[networking.EnvoyFilter_CLUSTER] {
		if cp.Operation != networking.EnvoyFilter_Patch_MERGE {
			continue
		}
		if commonConditionMatch(pctx, cp) && clusterMatch(c, cp) {

			if mergeTransportSocketCluster(c, cp) == false {
				proto.Merge(c, cp.Value)
			}
		}
	}
	return c
}

// Test if the patch contains a config for TransportSocket
func mergeTransportSocketCluster(c *cluster.Cluster, cp *model.EnvoyFilterConfigPatchWrapper) bool {

	cpValueCast, okCpCast := (cp.Value).(*cluster.Cluster)
	if !okCpCast {
		log.Errorf("ERROR Cast of cp.Value: %v", okCpCast)
		return false
	}

	if cpValueCast.GetTransportSocket() != nil {

		// Test if the cluster contains a config for TransportSocket
		if c.GetTransportSocketMatches() != nil {
			for transportSocketIndex, tsm := range c.TransportSocketMatches {
				if tsm.GetTransportSocket() != nil {
					if cpValueCast.TransportSocket.Name == tsm.TransportSocket.Name {

						// Merge the patch and the cluster at a lower level
						dstCluster := c.TransportSocketMatches[transportSocketIndex].TransportSocket.GetTypedConfig()
						srcPatch := cpValueCast.TransportSocket.GetTypedConfig()

						if dstCluster != nil && srcPatch != nil {

							retVal, err := util.MergeAnyWithAny(dstCluster, srcPatch)
							if err != nil {
								log.Errorf("ERROR merge any with any: %v", err)
								return false
							}

							// Merge the above result with the whole cluster
							proto.Merge(dstCluster, retVal)
							return true

						}
					}
				}
			}
		}
	}
	return false
}


func ShouldKeepCluster(pctx networking.EnvoyFilter_PatchContext, efw *model.EnvoyFilterWrapper, c *cluster.Cluster) bool {
	if efw == nil {
		return true
	}
	for _, cp := range efw.Patches[networking.EnvoyFilter_CLUSTER] {
		if cp.Operation != networking.EnvoyFilter_Patch_REMOVE {
			continue
		}
		if commonConditionMatch(pctx, cp) && clusterMatch(c, cp) {
			return false
		}
	}
	return true
}

func InsertedClusters(pctx networking.EnvoyFilter_PatchContext, efw *model.EnvoyFilterWrapper) []*cluster.Cluster {
	if efw == nil {
		return nil
	}
	var result []*cluster.Cluster
	// Add cluster if the operation is add, and patch context matches
	for _, cp := range efw.Patches[networking.EnvoyFilter_CLUSTER] {
		if cp.Operation == networking.EnvoyFilter_Patch_ADD {
			if commonConditionMatch(pctx, cp) {
				result = append(result, proto.Clone(cp.Value).(*cluster.Cluster))
			}
		}
	}
	return result
}

func clusterMatch(cluster *cluster.Cluster, cp *model.EnvoyFilterConfigPatchWrapper) bool {
	cMatch := cp.Match.GetCluster()
	if cMatch == nil {
		return true
	}

	if cMatch.Name != "" {
		return cMatch.Name == cluster.Name
	}

	_, subset, hostname, port := model.ParseSubsetKey(cluster.Name)

	if cMatch.Subset != "" && cMatch.Subset != subset {
		return false
	}

	if cMatch.Service != "" && host.Name(cMatch.Service) != hostname {
		return false
	}

	// FIXME: Ports on a cluster can be 0. the API only takes uint32 for ports
	// We should either make that field in API as a wrapper type or switch to int
	if cMatch.PortNumber != 0 && int(cMatch.PortNumber) != port {
		return false
	}
	return true
}
