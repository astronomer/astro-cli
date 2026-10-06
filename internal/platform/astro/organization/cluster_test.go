package organization

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/stretchr/testify/mock"

	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var mockClusterListResponse = astrov1.ListClustersResponse{
	HTTPResponse: &http.Response{StatusCode: 200},
	JSON200: &astrov1.ClustersPaginated{
		Clusters: []astrov1.Cluster{
			{
				Id:            "test-cluster-id",
				Name:          "test-cluster",
				CloudProvider: "AWS",
				Region:        "us-east-1",
				Type:          "DEDICATED",
				Status:        "CREATED",
			},
			{
				Id:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		},
		TotalCount: 2,
	},
}

// clusterOffsetIs matches ListClustersParams whose Offset equals want and
// whose pages are sorted by name, which keeps offset paging stable.
func clusterOffsetIs(want int) any {
	return mock.MatchedBy(func(p *astrov1.ListClustersParams) bool {
		return p != nil && p.Offset != nil && *p.Offset == want &&
			p.Sorts != nil && len(*p.Sorts) == 1 && (*p.Sorts)[0] == astrov1.ListClustersParamsSortsNameAsc
	})
}

func (s *Suite) TestListClusters() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	orgID := "test-org-id"

	s.Run("successful list all clusters", func() {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, orgID, mock.Anything).Return(&mockClusterListResponse, nil).Once()

		clusters, err := ListClusters(orgID, mockV1Client)
		s.NoError(err)
		s.Len(clusters, 2)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("paginates across pages advancing the offset", func() {
		page1 := astrov1.ListClustersResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrov1.ClustersPaginated{
				Clusters:   []astrov1.Cluster{{Id: "cluster1", Name: "cluster1"}},
				TotalCount: 2,
			},
		}
		page2 := astrov1.ListClustersResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200: &astrov1.ClustersPaginated{
				Clusters:   []astrov1.Cluster{{Id: "cluster2", Name: "cluster2"}},
				TotalCount: 2,
				Offset:     1,
			},
		}
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, orgID, clusterOffsetIs(0)).Return(&page1, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, orgID, clusterOffsetIs(1)).Return(&page2, nil).Once()

		clusters, err := ListClusters(orgID, mockV1Client)
		s.NoError(err)
		s.Len(clusters, 2)
		s.Equal("cluster1", clusters[0].Id)
		s.Equal("cluster2", clusters[1].Id)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("errors on a 200 with no parsed body", func() {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, orgID, mock.Anything).Return(&astrov1.ListClustersResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
		}, nil).Once()

		_, err := ListClusters(orgID, mockV1Client)
		s.ErrorContains(err, "empty response body")
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("error on listing clusters", func() {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListClustersResponse{}, errNetwork).Once()

		_, err := ListClusters(orgID, mockV1Client)
		s.ErrorIs(err, errNetwork)
		mockV1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestListClustersWithFormat() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("text output", func() {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockClusterListResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := ListClustersWithFormat(mockV1Client, testUtil.Renderer{Out: buf})
		s.NoError(err)
		s.Contains(buf.String(), "test-cluster")
		s.Contains(buf.String(), "us-east-1")
		s.Contains(buf.String(), "DEDICATED")
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("json output", func() {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockClusterListResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := ListClustersWithFormat(mockV1Client, testUtil.Renderer{JSON: true, Out: buf})
		s.NoError(err)

		var result ClusterList
		s.NoError(json.Unmarshal(buf.Bytes(), &result))
		s.Len(result.Clusters, 2)
		s.Equal("test-cluster", result.Clusters[0].Name)
		s.Equal("AWS", result.Clusters[0].CloudProvider)
		s.Equal("CREATED", result.Clusters[0].Status)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("empty list explains why", func() {
		emptyResponse := astrov1.ListClustersResponse{
			HTTPResponse: &http.Response{StatusCode: 200},
			JSON200:      &astrov1.ClustersPaginated{Clusters: []astrov1.Cluster{}},
		}
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := ListClustersWithFormat(mockV1Client, testUtil.Renderer{Out: buf})
		s.NoError(err)
		s.Contains(buf.String(), noClustersMsg)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("returns error on failure", func() {
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errNetwork).Once()

		buf := new(bytes.Buffer)
		err := ListClustersWithFormat(mockV1Client, testUtil.Renderer{Out: buf})
		s.ErrorIs(err, errNetwork)
		mockV1Client.AssertExpectations(s.T())
	})
}
