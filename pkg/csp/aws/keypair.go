//    Copyright 2018 Immutable Systems, Inc.
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func (a *awsService) KeyPairExist(ctx context.Context, keyName string) (bool, error) {
	if strings.TrimSpace(keyName) == "" {
		return false, nil
	}

	inputFilter := &ec2.DescribeKeyPairsInput{
		KeyNames: aws.StringSlice([]string{keyName}),
	}

	result, err := a.client.DescribeKeyPairsWithContext(ctx, inputFilter)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return false, ErrNotAllowed
		} else if ok && aerr.Code() == keyNotFoundErrorCode {
			return false, nil
		}
		return false, err
	}
	if result != nil && result.KeyPairs != nil {
		return len(result.KeyPairs) > 0, nil
	}
	return false, nil
}

func (a *awsService) CreateKeyPair(ctx context.Context, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", ErrInvalidName
	}
	if exist, _ := a.KeyPairExist(ctx, name); exist {
		return "", ErrInvalidName
	}
	input := &ec2.CreateKeyPairInput{
		KeyName: aws.String(name),
	}
	result, err := a.client.CreateKeyPairWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return "", ErrNotAllowed
		}
		return "", err
	}
	return *result.KeyMaterial, nil
}

func (a *awsService) RemoveKeyPair(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidName
	}
	input := &ec2.DeleteKeyPairInput{
		KeyName: aws.String(name),
	}
	_, err := a.client.DeleteKeyPairWithContext(ctx, input)
	if err != nil {
		aerr, ok := err.(awserr.Error)
		if ok && aerr.Code() == accessDeniedErrorCode {
			return ErrNotAllowed
		}
		return err
	}
	return nil
}
