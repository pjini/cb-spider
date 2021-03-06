// Proof of Concepts of CB-Spider.
// The CB-Spider is a sub-Framework of the Cloud-Barista Multi-Cloud Project.
// The CB-Spider Mission is to connect all the clouds with a single interface.
//
//      * Cloud-Barista: https://github.com/cloud-barista
//
// This is a Cloud Driver Example for PoC Test.
//
// program by ysjeon@mz.co.kr, 2019.07.
// modify by devunet@mz.co.kr, 2019.11.

package resources

import (
	"context"
	"errors"
	"strconv"
	"strings"

	compute "google.golang.org/api/compute/v1"

	"time"

	idrv "github.com/cloud-barista/cb-spider/cloud-control-manager/cloud-driver/interfaces"
	irs "github.com/cloud-barista/cb-spider/cloud-control-manager/cloud-driver/interfaces/resources"
	"github.com/davecgh/go-spew/spew"
)

type GCPVPCHandler struct {
	Region     idrv.RegionInfo
	Ctx        context.Context
	Client     *compute.Service
	Credential idrv.CredentialInfo
}

//@TODO : VPC 생성 로직 변경 필요 / 서브넷이 백그라운드로 생성되기 때문에 조회 시 모두 생성될 때까지 대기하는 로직 필요(그렇지 않으면 일부 정보가 누락됨)
func (vVPCHandler *GCPVPCHandler) CreateVPC(vpcReqInfo irs.VPCReqInfo) (irs.VPCInfo, error) {
	cblogger.Info(vpcReqInfo)

	var cnt string
	isFirst := false

	projectID := vVPCHandler.Credential.ProjectID
	region := vVPCHandler.Region.Region
	//name := GetCBDefaultVNetName()
	name := vpcReqInfo.IId.NameId

	//서브넷 있는지 확인
	autoCreateSubnetworks := false

	if vpcReqInfo.SubnetInfoList == nil {
		autoCreateSubnetworks = true
	}

	cblogger.Infof("생성된 [%s] VPC가 있는지 체크", name)
	vNetInfo, errVnet := vVPCHandler.Client.Networks.Get(projectID, name).Do()
	spew.Dump(vNetInfo)
	if errVnet != nil {
		isFirst = true
		cblogger.Error(errVnet)

		cblogger.Infof("존재하는 [%s] VPC가 없으므로 새로 생성해야 함", name)
		network := &compute.Network{
			Name: name,
			//Name:                  GetCBDefaultVNetName(),
			AutoCreateSubnetworks: autoCreateSubnetworks, // subnet 유무에 따라서 달라짐 true, false
			ForceSendFields:       []string{"AutoCreateSubnetworks"},
		}

		cblogger.Infof("[%s] VPC 생성 시작", name)
		cblogger.Info(network)
		_, err := vVPCHandler.Client.Networks.Insert(projectID, network).Do()
		if err != nil {
			cblogger.Errorf("[%s] VPC 생성 실패", name)
			cblogger.Error(err)
			return irs.VPCInfo{}, errVnet
		}

		cblogger.Infof("[%s] VPC 정상적으로 생성되고 있습니다.", name)
		before_time := time.Now()
		max_time := 120

		// loop --> 생성 check --> 생성 되었으면, break; 안됐으면 sleep 5초 -->
		// if(total sleep 120sec?) error

		cblogger.Info("VPC가 모두 생성될 때까지 5초 텀으로 체크 시작")
		for {
			cblogger.Infof("==> [%s] VPC 정보 조회", name)
			_, errVnet := vVPCHandler.Client.Networks.Get(projectID, name).Do()
			if errVnet != nil {
				cblogger.Errorf("==> [%s] VPC 정보 조회 실패", name)
				cblogger.Error(errVnet)

				time.Sleep(time.Second * 5)
				after_time := time.Now()
				diff := after_time.Sub(before_time)
				if int(diff.Seconds()) > max_time {
					cblogger.Errorf("[%d]초 동안 [%s] VPC 정보가 조회되지 않아서 강제로 종료함.", max_time, name)
					return irs.VPCInfo{}, errVnet
				}
			} else {
				//생성된 VPC와 서브넷 이름이 동일하지 않으면 VPC의 기본 서브넷이 모두 생성될 때까지 20초 정도 대기
				//if name != VPCReqInfo.Name {
				cblogger.Info("생성된 VPC정보가 조회되어도 리전에서는 계속 생성되고 있기 때문에 20초 대기")
				time.Sleep(time.Second * 20)
				//}

				cblogger.Infof("==> [%s] VPC 정보 생성 완료", name)
				//서브넷이 비동기로 생성되고 있기 때문에 다시 체크해야 함.
				newvNetInfo, _ := vVPCHandler.Client.Networks.Get(projectID, name).Do()
				cnt = strconv.Itoa(len(newvNetInfo.Subnetworks) + 1)
				break
			}
		}
	} else {
		cblogger.Infof("이미 [%s] VPCs가 존재함.", name)
		cnt = strconv.Itoa(len(vNetInfo.Subnetworks) + 1)
		//return irs.VPCInfo{}, errors.New(name + " VPC가 존재하기 때문에 생서할 수 없습니다.")
		return irs.VPCInfo{}, errors.New("Already Exist - " + vpcReqInfo.IId.NameId)
	}

	cblogger.Info("현재 생성된 서브넷 수 : ", cnt)
	vpcNetworkUrl := "https://www.googleapis.com/compute/v1/projects/" + projectID + "/global/networks/" + vpcReqInfo.IId.NameId
	// 여기서부터 서브넷 체크하는 로직이 들어가야 하네. 하필 배열이네
	for _, item := range vpcReqInfo.SubnetInfoList {
		subnetName := item.IId.NameId
		cblogger.Infof("생성할 [%s] Subnet이 존재하는지 체크", subnetName)
		checkInfo, err := vVPCHandler.Client.Subnetworks.Get(projectID, region, subnetName).Do()
		if err == nil {
			cblogger.Error("이미 해당하는 Subnet이 존재함")
			return irs.VPCInfo{}, err
		}
		cblogger.Info(" Subnet info : ", checkInfo)

		subnetWork := &compute.Subnetwork{
			Name:        subnetName,
			IpCidrRange: item.IPv4_CIDR,
			Network:     vpcNetworkUrl,
		}
		cblogger.Infof("[%s] Subnet 생성시작", subnetName)
		cblogger.Info(subnetWork)

		infoSubnet, errSubnet := vVPCHandler.Client.Subnetworks.Insert(projectID, region, subnetWork).Do()
		if errSubnet != nil {
			cblogger.Error(errSubnet)
			return irs.VPCInfo{}, errors.New("Making Subnet Error - " + subnetName)
		}

		cblogger.Infof("[%s] Subnet 생성완료", subnetName)
		cblogger.Info(infoSubnet)

	}

	//생성된 서브넷이 모두 조회되는데 시간이 필요 함.
	cblogger.Info("waiting 5 seconds for subnet info")
	time.Sleep(time.Second * 5)

	vpcInfo, errVPC := vVPCHandler.GetVPC(irs.IID{SystemId: vpcReqInfo.IId.NameId})
	if errVPC != nil {
		cblogger.Error("VPC 생성 후 생성된 정보 조회 시 에러 발생 - isFirst : ", isFirst)
		cblogger.Error(errVPC)
		return vpcInfo, errVPC
	}
	/*
		if errVPC == nil {
			spew.Dump(vpcInfo)
			//최초 생성인 경우 VPC와 Subnet 이름이 동일하면 이미 생성되었으므로 추가로 생성하지 않고 리턴 함.
			if isFirst {
				cblogger.Error("최초 VPC 생성이므로 에러 없이 조회된 서브넷 정보를 리턴 함.")
				return vpcInfo, nil
			} else {
				cblogger.Error(errVPC)
				return irs.VPCInfo{}, errors.New("Already Exist - " + vpcReqInfo.IId.NameId)
			}
		}
	*/

	vpcInfo.IId.NameId = vpcReqInfo.IId.NameId

	//생성되는데 시간이 필요 함. 약 20초정도?
	//time.Sleep(time.Second * 20)

	return vpcInfo, nil
}

func (vVPCHandler *GCPVPCHandler) ListVPC() ([]*irs.VPCInfo, error) {
	projectID := vVPCHandler.Credential.ProjectID
	//region := vVPCHandler.Region.Region

	vpcList, err := vVPCHandler.Client.Networks.List(projectID).Do()

	if err != nil {

		return nil, err
	}
	var vpcInfo []*irs.VPCInfo

	for _, item := range vpcList.Items {
		iId := irs.IID{
			NameId: item.Name,
			//SystemId: strconv.FormatUint(item.Id, 10),
			SystemId: item.Name,
		}
		subnetInfo, err := vVPCHandler.GetVPC(iId)
		if err != nil {
			cblogger.Error(err)
			return vpcInfo, err
		}

		vpcInfo = append(vpcInfo, &subnetInfo)

	}

	return vpcInfo, nil
}

func (vVPCHandler *GCPVPCHandler) GetVPC(vpcIID irs.IID) (irs.VPCInfo, error) {

	projectID := vVPCHandler.Credential.ProjectID
	region := vVPCHandler.Region.Region
	//name := VPCID
	systemId := vpcIID.SystemId
	//name := vpcIID.NameId

	//cblogger.Infof("NameID : [%s] / SystemID : [%s]", name, systemId)
	cblogger.Infof("SystemID : [%s]", systemId)
	subnetInfoList := []irs.SubnetInfo{}

	infoVPC, err := vVPCHandler.Client.Networks.Get(projectID, systemId).Do()
	//infoVPC, err := vVPCHandler.Client.Networks.Get(projectID, name).Do()
	if err != nil {
		cblogger.Error(err)
		return irs.VPCInfo{}, err
	}
	spew.Dump(infoVPC)
	if infoVPC.Subnetworks != nil {
		for _, item := range infoVPC.Subnetworks {
			str := strings.Split(item, "/")
			region = str[len(str)-3]
			subnet := str[len(str)-1]
			infoSubnet, err := vVPCHandler.Client.Subnetworks.Get(projectID, region, subnet).Do()
			if err != nil {
				cblogger.Error(err)
				return irs.VPCInfo{}, err
			}
			subnetInfoList = append(subnetInfoList, mappingSubnet(infoSubnet))
		}

	}

	networkInfo := irs.VPCInfo{
		IId: irs.IID{
			NameId: infoVPC.Name,
			//SystemId: strconv.FormatUint(infoVPC.Id, 10),
			SystemId: infoVPC.Name,
		},
		IPv4_CIDR:      "Not support IPv4_CIDR at GCP VPC",
		SubnetInfoList: subnetInfoList,
		KeyValueList: []irs.KeyValue{
			{"RoutingMode", infoVPC.RoutingConfig.RoutingMode},
			{"Description", infoVPC.Description},
			{"SelfLink", infoVPC.SelfLink},
		},
	}

	return networkInfo, nil
}

func mappingSubnet(subnet *compute.Subnetwork) irs.SubnetInfo {
	//str := subnet.SelfLink
	str := strings.Split(subnet.SelfLink, "/")
	subnetName := str[len(str)-1]
	regionStr := strings.Split(subnet.Region, "/")
	region := regionStr[len(regionStr)-1]
	subnetInfo := irs.SubnetInfo{
		IId: irs.IID{
			NameId: subnet.Name,
			//SystemId: strconv.FormatUint(subnet.Id, 10),
			SystemId: subnet.Name,
		},
		IPv4_CIDR: subnet.IpCidrRange,
		KeyValueList: []irs.KeyValue{
			{"region", region},
			{"subnet", subnetName},
		},
	}
	return subnetInfo
}

func (vVPCHandler *GCPVPCHandler) DeleteVPC(vpcID irs.IID) (bool, error) {
	projectID := vVPCHandler.Credential.ProjectID
	//region := vVPCHandler.Region.Region
	//name := VPCID
	//name := vpcID.NameId
	name := vpcID.SystemId
	cblogger.Infof("Name : [%s]", name)
	subnetInfo, subErr := vVPCHandler.GetVPC(vpcID)
	if subErr != nil {
		cblogger.Error(subErr)
		return false, subErr
	}
	if subnetInfo.SubnetInfoList != nil {
		for _, item := range subnetInfo.SubnetInfoList {
			for _, si := range item.KeyValueList {
				if si.Key == "region" {
					region := si.Value
					infoSubnet, infoSubErr := vVPCHandler.Client.Subnetworks.Delete(projectID, region, item.IId.NameId).Do()
					if infoSubErr != nil {
						cblogger.Error(infoSubErr)
						return false, infoSubErr
					}
					cblogger.Info("Delete subnet result :", infoSubnet)
					cblogger.Info("Subnet Deleting....wait 10seconds")
					time.Sleep(time.Second * 10)
				}
			}
		}
	}
	cblogger.Info("VPC Deleting....wait 15seconds")
	time.Sleep(time.Second * 15)
	cblogger.Info("[NOW Delete VPC]")
	info, err := vVPCHandler.Client.Networks.Delete(projectID, name).Do()
	time.Sleep(time.Second * 15)
	cblogger.Info(info)
	if err != nil {
		cblogger.Error(err)
		return false, err
	}
	//fmt.Println(info)
	return true, nil
}
