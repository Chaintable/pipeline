package util

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Chaintable/pipeline/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/gzip"
)

func TestS3(t *testing.T) {
	s3client, err := NewS3Client("ap-northeast-1")
	if err != nil {
		t.Errorf("Expected s3 client, got nil")
	}
	// res, err := s3client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	// if err != nil {
	// 	t.Errorf("Expected error, got nil")
	// }
	// if res == nil {
	// 	t.Errorf("Expected response, got nil")
	// }
	// t.Logf("Tested NewS3Client() successfully, %+v", res)
	b, err := DownloadFileFromS3(s3client, "chaintable-pipeline-test", "1/0xaeaa51f5cf80c31e3c21a80e5b2399e31675fe253c676d351db66aa35d341374")
	if err != nil {
		t.Errorf("Expected error, got nil")
	}
	decodeStartTime := time.Now()
	file := types.BlockFile{}
	err = DecodeFromGzipJson(b, &file)
	if err != nil {
		t.Errorf("Expected error, got nil")
	}
	t.Logf("DecodeFromGzipJson() took %v", time.Since(decodeStartTime))

	encodeStartTime := time.Now()
	// t.Logf("Tested DownloadFileFromS3() successfully, %+v", file)
	EncodeToJsonGzip(file)
	t.Logf("EncodeToJsonGzip() took %v", time.Since(encodeStartTime))
	// t.Logf("Tested DownloadFileFromS3() successfully, %+v", file)
}

func TestS31(t *testing.T) {
	s3client, err := NewS3Client("ap-northeast-1")
	if err != nil {
		t.Errorf("Expected s3 client, got nil")
	}

	// res, err := s3client.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
	// 	Bucket: aws.String("chaintable-pipeline--apne1-az4--x-s3"),
	// 	Prefix: aws.String("8453/27614340/"),
	// })
	// if err != nil {
	// 	t.Errorf("Expected error, got nil")
	// }
	// if res == nil {
	// 	t.Errorf("Expected response, got nil")
	// }

	// t.Logf("Tested NewS3Client() successfully, %+v", len(res.Contents))
	// res, err := s3client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	// if err != nil {
	// 	t.Errorf("Expected error, got nil")
	// }
	// if res == nil {
	// 	t.Errorf("Expected response, got nil")
	// }
	// t.Logf("Tested NewS3Client() successfully, %+v", res)
	b, err := DownloadFileFromS3(s3client, "chaintable-pipeline--apne1-az4--x-s3", "0/0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3")
	if err != nil {
		t.Errorf("Expected error, got nil, err %v", err)
		return
	}
	gz, err := gzip.NewReader(bytes.NewBuffer(b))
	if err != nil {
		t.Errorf("err %v", err)
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(gz)
	// // write to local file
	f, err := os.Create("test.json")
	if err != nil {
		t.Errorf("err %v", err)
	}
	defer f.Close()
	_, err = f.Write(buf.Bytes())
	if err != nil {
		t.Errorf("err %v", err)
	}
	// t.Logf("Tested DownloadFileFromS3() successfully, %+v", gz)

	// blockDiff := new(types.BlockFile)
	// err = DecodeFromGzipJson(b, blockDiff)
	// if err != nil {
	// 	t.Errorf("Expected error, got nil")
	// }
	// t.Logf("Tested DecodeFromRlp() successfully, %+v", blockDiff)

}

func TestS32(t *testing.T) {
	s3client, err := NewS3Client("ap-northeast-1")
	if err != nil {
		t.Errorf("Expected s3 client, got nil")
	}
	b, err := DownloadFileFromS3(s3client, "chaintable-nodex-pipeline--apne1-az4--x-s3", "9745/0x795f3d892544c253078f1a219dea1931cb8b874e7f50c1c9a2f451166e4d26c3/stateDiff")
	if err != nil {
		t.Errorf("Expected error, got nil, err %v", err)
		return
	}
	blockDiff := new(types.BlockStorageDiff)
	err = DecodeFromRlp(b, blockDiff)
	if err != nil {
		t.Errorf("Expected error, got %v", err)
	}
	t.Logf("Tested DecodeFromRlp() successfully, %+v", blockDiff)
	f, err := os.Create("test_statediff.json")
	if err != nil {
		t.Errorf("err %v", err)
	}
	defer f.Close()
	buf, _ := json.Marshal(blockDiff)
	_, err = f.Write(buf)
	if err != nil {
		t.Errorf("err %v", err)
	}

}

func TestHex(t *testing.T) {
	b, err := hex.DecodeString("f910e5a08fbafb1b013239d71d5290a91e90b944a2cfc40560ef86161c51187525fac445a0ce208f4817c54212f86816ca10f086dfe00552f224e20125654c23df5b40d5f8f9017bf850a0d837f019de32cc5a1af198343e0f1adea565b33b12da13dce0c0fcb2d6c2a4888c0cecb8d204e6987668394f8c01a0d7d408ebcd99b2b70be43e20253d6d92a8ea8fab29bd3be7f55b10032331fb4cf84ca0d16443a28465b028cdebe5b857d70e749e69e1f31856fbe900349d06ee580a4c880de0b6b3a764000001a047e6835a0a22b0d581faf0543c650db2190295e3ef4b9c442895b72e2b7dc056f844a037d65eaa92c6bc4c13a5ec45527f0c18ea8932588728769ec7aecfe6d9f32e428001a0f57acd40259872606d76197ef052f3d35588dadf919ee1f0e3cb9b62d3f4b02cf84da0ba4c454e7c2a52bdd9ed459778501b628c40b5dfc0669ed42d418512e5f10c518903fbf9ae46806b33aa05a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470f844a06c9d57be05dd69371c4dd2e871bce6e9f4124236825bb612ee18a45e5675be518001a06e49e66782037c0555897870e29fa5e552daf4719552131a0abce779daec0a5dc0f90174f879a0d16443a28465b028cdebe5b857d70e749e69e1f31856fbe900349d06ee580a4cf856eaa04900f50c68fa681d9912d53052a0b056573eec80a1e83f421fbdd51efa8000e38806f05b59d3b20000eaa031550336dd318a9aafe86bef1d682a371e625956f44bb393a681049df0f2a8b38806f05b59d3b20000f88ea037d65eaa92c6bc4c13a5ec45527f0c18ea8932588728769ec7aecfe6d9f32e42f86be6a0fb29f85d5cf675a5700fdf4ee53edd4d3976fb39785e3cc0a23529ab9171cd2b8468c044c9f842a09e45a068ecca232d492d494be15f0891905b5816ee8f81b86d5f7db10dba966ea0586b5d9daeec94c821a1f794d79719dac61330add1b34811a0b083d9877df3c1f867a06c9d57be05dd69371c4dd2e871bce6e9f4124236825bb612ee18a45e5675be51f844f842a09407a07b64f896f2307ac26bde68fa50501bdec72d49032d2274b3422cdce32ca016216bd66b7e0c8908b2b6fef3b27d323e94743d9ce91b3989102911ebbfce4bf90daaf90ca9a047e6835a0a22b0d581faf0543c650db2190295e3ef4b9c442895b72e2b7dc056b90c856080604052600436106100af576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806306fdde03146100b9578063095ea7b31461014957806318160ddd146101ae57806323b872dd146101d95780632e1a7d4d1461025e578063313ce5671461028b57806370a08231146102bc57806395d89b4114610313578063a9059cbb146103a3578063d0e30db014610408578063dd62ed3e14610412575b6100b7610489565b005b3480156100c557600080fd5b506100ce610526565b6040518080602001828103825283818151815260200191508051906020019080838360005b8381101561010e5780820151818401526020810190506100f3565b50505050905090810190601f16801561013b5780820380516001836020036101000a031916815260200191505b509250505060405180910390f35b34801561015557600080fd5b50610194600480360381019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803590602001909291905050506105c4565b604051808215151515815260200191505060405180910390f35b3480156101ba57600080fd5b506101c36106b6565b6040518082815260200191505060405180910390f35b3480156101e557600080fd5b50610244600480360381019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803590602001909291905050506106d5565b604051808215151515815260200191505060405180910390f35b34801561026a57600080fd5b5061028960048036038101908080359060200190929190505050610a22565b005b34801561029757600080fd5b506102a0610b55565b604051808260ff1660ff16815260200191505060405180910390f35b3480156102c857600080fd5b506102fd600480360381019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610b68565b6040518082815260200191505060405180910390f35b34801561031f57600080fd5b50610328610b80565b6040518080602001828103825283818151815260200191508051906020019080838360005b8381101561036857808201518184015260208101905061034d565b50505050905090810190601f1680156103955780820380516001836020036101000a031916815260200191505b509250505060405180910390f35b3480156103af57600080fd5b506103ee600480360381019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190505050610c1e565b604051808215151515815260200191505060405180910390f35b610410610489565b005b34801561041e57600080fd5b50610473600480360381019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610c33565b6040518082815260200191505060405180910390f35b34600360003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020600082825401925050819055503373ffffffffffffffffffffffffffffffffffffffff167fe1fffcc4923d04b559f4d29a8bfc6cda04eb5b0d3c460751c2402c5c5cc9109c346040518082815260200191505060405180910390a2565b60008054600181600116156101000203166002900480601f0160208091040260200160405190810160405280929190818152602001828054600181600116156101000203166002900480156105bc5780601f10610591576101008083540402835291602001916105bc565b820191906000526020600020905b81548152906001019060200180831161059f57829003601f168201915b505050505081565b600081600460003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508273ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff167f8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925846040518082815260200191505060405180910390a36001905092915050565b60003073ffffffffffffffffffffffffffffffffffffffff1631905090565b600081600360008673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020541015151561072557600080fd5b3373ffffffffffffffffffffffffffffffffffffffff168473ffffffffffffffffffffffffffffffffffffffff16141580156107fd57507fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff600460008673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205414155b156109185781600460008673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020541015151561088d57600080fd5b81600460008673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020600082825403925050819055505b81600360008673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000206000828254039250508190555081600360008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020600082825401925050819055508273ffffffffffffffffffffffffffffffffffffffff168473ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef846040518082815260200191505060405180910390a3600190509392505050565b80600360003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410151515610a7057600080fd5b80600360003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020600082825403925050819055503373ffffffffffffffffffffffffffffffffffffffff166108fc829081150290604051600060405180830381858888f19350505050158015610b03573d6000803e3d6000fd5b503373ffffffffffffffffffffffffffffffffffffffff167f7fcf532c15f0a6db0bd6d0e038bea71d30d808c7d98cb3bf7268a95bf5081b65826040518082815260200191505060405180910390a250565b600260009054906101000a900460ff1681565b60036020528060005260406000206000915090505481565b60018054600181600116156101000203166002900480601f016020809104026020016040519081016040528092919081815260200182805460018160011615610100020316600290048015610c165780601f10610beb57610100808354040283529160200191610c16565b820191906000526020600020905b815481529060010190602001808311610bf957829003601f168201915b505050505081565b6000610c2b3384846106d5565b905092915050565b60046020528160005260406000206020528060005260406000206000915091505054815600a165627a7a7230582036bcecaf8d1ecbc876518b43d906e443817b0a63156c5fd2337da8d3d0707614002900f876a06e49e66782037c0555897870e29fa5e552daf4719552131a0abce779daec0a5db8533373fffffffffffffffffffffffffffffffffffffffe14604657602036036042575f35600143038111604257611fff81430311604257611fff9006545f5260205ff35b5f5ffd5b5f35611fff60014303065500f884a0f57acd40259872606d76197ef052f3d35588dadf919ee1f0e3cb9b62d3f4b02cb8613373fffffffffffffffffffffffffffffffffffffffe14604d57602036146024575f5ffd5b5f35801560495762001fff810690815414603c575f5ffd5b62001fff01545f5260205ff35b5f5ffd5b62001fff42064281555f359062001fff015500")
	if err != nil {
		t.Errorf("err %v", err)
	}
	blockDiff := new(types.BlockStorageDiff)
	err = DecodeFromRlp(b, blockDiff)
	if err != nil {
		t.Errorf("Expected error, got %v", err)
	}
	t.Logf("Tested DecodeFromRlp() successfully, %+v", blockDiff)
}

func TestS323(t *testing.T) {
	s3client, err := NewS3Client("ap-northeast-1")
	if err != nil {
		t.Errorf("Expected s3 client, got nil")
	}
	bucket := "chaintable-pipeline--apne1-az4--x-s3"
	prefix := "100/41743607/"
	rsp, err := s3client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket: &bucket,
		Prefix: &prefix,
	})
	for _, c := range rsp.Contents {
		t.Logf("Found object: %s", *c.Key)
		b, err := DownloadFileFromS3(s3client, bucket, *c.Key)
		if err != nil {
			t.Errorf(" err %v", err)
			return
		}
		blockDiff := new(types.BlockValidation)
		err = DecodeFromGzipJson(b, blockDiff)
		if err != nil {
			t.Errorf("Not Expected error, got %v", err)
			return
		}
		t.Logf("Tested DecodeFromGzipJson() successfully, %+v", blockDiff)
	}
}

func TestS322(t *testing.T) {
	s3client, err := NewS3Client("ap-northeast-1")
	if err != nil {
		t.Errorf("Expected s3 client, got nil")
	}
	b, err := DownloadFileFromS3(s3client, "chaintable-nodex-pipeline--apne1-az4--x-s3", "9745/0xdfe50ba96e62078f644a85cda20d37a2746b93e1cc0efe064cae41f4ac095bc2/block")
	if err != nil {
		t.Errorf("Expected error, got nil, err %v", err)
		return
	}
	blockDiff := new(types.Header)
	err = DecodeFromGzipJson(b, blockDiff)
	if err != nil {
		t.Errorf("Not Expected error, got %v", err)
		return
	}
	f, err := os.Create("test.json")
	if err != nil {
		t.Errorf("err %v", err)
	}
	defer f.Close()
	buf, _ := json.Marshal(blockDiff)
	_, err = f.Write(buf)
	if err != nil {
		t.Errorf("err %v", err)
	}
}

func TestS33(t *testing.T) {
	s3client, err := NewS3Client("ap-northeast-1")
	if err != nil {
		t.Errorf("Expected s3 client, got nil")
	}

	b, err := DownloadFileFromS3(s3client, "chaintable-pipeline--apne1-az4--x-s3", "59144/0x2614f0daaccde8664fd6bbc192183afa62739e9a6590f7e5144372a697b6de28")
	if err != nil {
		t.Errorf("Expected error, got nil, err %v", err)
		return
	}
	blockFile := new(types.BlockFile)

	err = DecodeFromGzipJson(b, blockFile)
	if err != nil {
		t.Errorf("Not Expected error, got %v", err)
		return
	}

	t.Logf("Tested DecodeFromGzipJson() successfully, %+v", blockFile)

	gz, err := gzip.NewReader(bytes.NewBuffer(b))
	if err != nil {
		t.Errorf("err %v", err)
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(gz)
	// write to local file
	f, err := os.Create("test.json")
	if err != nil {
		t.Errorf("err %v", err)
	}
	defer f.Close()
	_, err = f.Write(buf.Bytes())
	if err != nil {
		t.Errorf("err %v", err)
	}

}

func TestS34(t *testing.T) {
	s := "f90420a0c928a14ea6eb339069704871e67bc633a93f9b1d78d256f5d5a13009afe2bc83a0405725e649ecd6047db836c2eaec5a4b216d3cdd7e837468e2c5f373bd9994eff901b8f844a0686bcf03a9d022ca5775fc88c2a5c218e07b6da6392bdd7b263d4f678567b3a78001a0e5e3693157141608a301682c8c228c0277eac7efc0b98b57f874ca49752b5fd8f844a037d65eaa92c6bc4c13a5ec45527f0c18ea8932588728769ec7aecfe6d9f32e428001a0f57acd40259872606d76197ef052f3d35588dadf919ee1f0e3cb9b62d3f4b02cf844a00d76b57820feb615f13c61cac840e9ef46eed2235c652b1f90459f5d87c048ac8002a0d6acad7edd8b31eb2486956db885fc0ad1293dc054073e942f2dc75539ed6d34f844a06c9d57be05dd69371c4dd2e871bce6e9f4124236825bb612ee18a45e5675be518001a06e49e66782037c0555897870e29fa5e552daf4719552131a0abce779daec0a5df84ca0b27d3996258523b04f6387d5e2ab158c40e2cc70c75644f1f6af3242a88fe9aa8806b11eeb0ff8cdfa13a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470f850a0d837f019de32cc5a1af198343e0f1adea565b33b12da13dce0c0fcb2d6c2a4888c0cecb8d20924138c177734d601a0d7d408ebcd99b2b70be43e20253d6d92a8ea8fab29bd3be7f55b10032331fb4cc0f9021ef8c8a0686bcf03a9d022ca5775fc88c2a5c218e07b6da6392bdd7b263d4f678567b3a7f8a5f6a075b20eef8615de99c108b05f0dbda081c91897128caa336d75dffb97c4132b4d94609f3624bf8404a1b31abea037b6256acfe421d3f6a052df0bdf5a5f92d8037cf11e50f13d8017aefc99d20a73c826416df79570d481942c69136a23cc4d88bbb7e805992e5a2d25095c77f6a047f5c767712c0cb55e0943de27dbb2db3b8bdac1e899af22d477462089a99558941a6362ad64ccff5902d46d875b36e8798267d154f88ea037d65eaa92c6bc4c13a5ec45527f0c18ea8932588728769ec7aecfe6d9f32e42f86be6a0468489391be3d5074f6b49973a77086b22ce78a0e253b0c7d2b2bfcd182c684d8468c08c09f842a09eff1148258896a11476ac92eadcb187a00114f825b2794e517b14d5480e9d5ca0b6eddfc2918a38ba72d1d7adcf6cf6fb996343d02ca4a892387c285410a25abef859a00d76b57820feb615f13c61cac840e9ef46eed2235c652b1f90459f5d87c048acf7f6a07382eac8ad4c385fe72789298821c0c29a729e94132b01652a33a57e1821993d944dff9b5b0143e642a3f63a5bcf2d1c328e600bf8f867a06c9d57be05dd69371c4dd2e871bce6e9f4124236825bb612ee18a45e5675be51f844f842a0a785c89cf171c4e31fe8d8e27f5fc4659d03951421b6decb067977b1e24787b2a0182902ee1cd0321dbdd7f42ca620e7d3dfa219e0c435a7d4fb7c03b4d05e7365c0"
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Errorf("Expected error, got nil, err %v", err)
		return
	}
	blockDiff := new(types.BlockStorageDiff)
	err = DecodeFromRlp(b, blockDiff)
	if err != nil {
		t.Errorf("Expected error, got %v", err)
	}
	t.Logf("Tested DecodeFromRlp() successfully, %+v", blockDiff)
	f, err := os.Create("test2.json")
	if err != nil {
		t.Errorf("err %v", err)
	}
	defer f.Close()
	buf, _ := json.Marshal(blockDiff)
	_, err = f.Write(buf)
	if err != nil {
		t.Errorf("err %v", err)
	}
}
