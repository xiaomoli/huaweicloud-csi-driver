package main

import (
	"errors"
	"fmt"
	"github.com/chnsz/golangsdk"
	"github.com/chnsz/golangsdk/openstack"
	"github.com/chnsz/golangsdk/openstack/sfs/v2/shares"
	"github.com/golang/glog"
	"github.com/xiaomoli/huaweicloud-csi-driver/pkg/sfs/config"
	"gopkg.in/gcfg.v1"
	"k8s.io/klog"
	"net/http"
	"os"
)

const (
	waitForAvailableShareTimeout = 3

	shareAvailable = "available"

	shareDescription = "provisioned-by=sfs.csi.huaweicloud.org"
)

// CloudCredentials define
type CloudCredentials struct {
	Global struct {
		AccessKey string `gcfg:"access-key"`
		SecretKey string `gcfg:"secret-key"`
		Region    string `gcfg:"region"`
		AuthURL   string `gcfg:"auth-url"`
	}

	Vpc struct {
		Id string `gcfg:"id"`
	}

	CloudClient *golangsdk.ProviderClient
}

// LoadConfig from file

func LoadConfig(configFile string) (cc CloudCredentials, err error) {
	//Check file path
	if configFile == "" {
		return cc, errors.New("Must provide a config file")
	}

	// Get config from file
	glog.Infof("load config from file: %s", configFile)
	file, err := os.Open(configFile)
	if err != nil {
		return cc, err
	}
	defer file.Close()

	// Read configuration
	err = gcfg.FatalOnly(gcfg.ReadInto(&cc, file))
	if err != nil {
		return cc, err
	}

	// Validate configuration
	err = cc.Validate()
	if err != nil {
		return cc, err
	}

	return cc, nil
}

// Validate configuration
func (cc *CloudCredentials) Validate() error {
	ao := golangsdk.AKSKAuthOptions{
		IdentityEndpoint: cc.Global.AuthURL,
		AccessKey:        cc.Global.AccessKey,
		SecretKey:        cc.Global.SecretKey,
		ProjectName:      cc.Global.Region,
	}
	client, err := openstack.NewClient(ao.IdentityEndpoint)
	if err != nil {
		return err
	}
	// if OS_DEBUG is set, log the requests and responses
	var osDebug bool
	if os.Getenv("OS_DEBUG") != "" {
		osDebug = true
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	client.HTTPClient = http.Client{
		Transport: &config.LogRoundTripper{
			Rt:      transport,
			OsDebug: osDebug,
		},
	}

	err = openstack.Authenticate(client, ao)
	if err != nil {
		return err
	}

	cc.CloudClient = client
	return nil
}

func main() {

	//cloudconfig and get auth
	cloudconfig := "D:\\coding\\xiaomo\\huaweicloud-csi-driver\\cmd\\sfs-test\\cloud-config"
	cloud, err := LoadConfig(cloudconfig)

	if err != nil {
		klog.V(3).Infof("Failed to load cloud config: %v", err)
	}
	//

	//client, err := cloud.SFSV2Client()
	client, err := openstack.NewSharedFileSystemV2(cloud.CloudClient, golangsdk.EndpointOpts{
		Region:       cloud.Global.Region,
		Availability: golangsdk.AvailabilityPublic,
	})

	if err != nil {
		fmt.Println(err)
	}

	////create
	//volume, err := createShare(client, &shares.CreateOpts{
	//	ShareProto:  "NFS",
	//	Size:        10,
	//	Name:        "k8s-test1",
	//	Description: "test test",
	//})
	//if err != nil {
	//	fmt.Println(err)
	//}
	//fmt.Printf("创建的share：%v\n", volume)

	////get
	ID := "9c96c76f-b0e1-49c2-81b8-b4fc63bdd790"
	volume2, err := getShare(client, ID)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("查询到的share：%v\n", volume2)
	////extend
	//err = expandShare(client, ID, 20)
	//if err != nil {
	//	fmt.Println(err)
	//}
	//volume3, err := getShare(client, ID)
	//if err != nil {
	//	fmt.Println(err)
	//}
	//fmt.Printf("查询到的share new：%v\n", volume3)
	//
	//delete
	err = deleteShare(client, ID)
	if err != nil {
		fmt.Println(err)
	}
	volume4, err := getShare(client, ID)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("查询到的share del：%v\n", volume4)
}

//delete
func deleteShare(client *golangsdk.ServiceClient, shareID string) error {
	if err := shares.Delete(client, shareID).ExtractErr(); err != nil {
		if _, ok := err.(golangsdk.ErrDefault404); ok {
			klog.V(4).Infof("share %s not found, assuming it to be already deleted", shareID)
		} else {
			return err
		}
	}

	return nil
}

//expand
func expandShare(client *golangsdk.ServiceClient, shareID string, size int) error {
	expandOpts := shares.ExpandOpts{OSExtend: shares.OSExtendOpts{NewSize: size}}
	expand := shares.Expand(client, shareID, expandOpts)
	return expand.Err
}

//get
func getShare(client *golangsdk.ServiceClient, shareID string) (*shares.Share, error) {
	return shares.Get(client, shareID).Extract()
}

//create
func createShare(client *golangsdk.ServiceClient, opts *shares.CreateOpts) (*shares.Share, error) {
	//幂等性怎么保证

	share, err := shares.Create(client, opts).Extract()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(share.ID, share.ProjectID, share.Status, share.AvailabilityZone)
	// wait
	err = waitForShareStatus(client, share.ID, shareAvailable, waitForAvailableShareTimeout)
	return share, err
}

func waitForShareStatus(client *golangsdk.ServiceClient, shareID string, desiredStatus string, timeout int) error {
	return golangsdk.WaitFor(timeout, func() (bool, error) {
		share, err := shares.Get(client, shareID).Extract()
		if err != nil {
			return false, err
		}
		return share.Status == desiredStatus, nil
	})
}
