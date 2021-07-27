package main

import (
	"github.com/aws/aws-cdk-go/awscdk"
	"github.com/aws/aws-cdk-go/awscdk/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/awsecrassets"
	"github.com/aws/aws-cdk-go/awscdk/awsecs"
	ecspatterns "github.com/aws/aws-cdk-go/awscdk/awsecspatterns"
	"github.com/aws/aws-cdk-go/awscdk/awsefs"
	"github.com/aws/aws-cdk-go/awscdk/awselasticloadbalancingv2"
	"github.com/aws/aws-cdk-go/awscdk/awsrds"
	"github.com/aws/constructs-go/constructs/v3"
	"github.com/aws/jsii-runtime-go"
)

type StrapiStackProps struct {
	awscdk.StackProps
}

func NewStrapiStack(scope constructs.Construct, id string, props *StrapiStackProps) awscdk.Stack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	// create vpc
	vpc := awsec2.NewVpc(stack, jsii.String("StrapiVpc"),
		&awsec2.VpcProps{
			MaxAzs: jsii.Number(2),
			SubnetConfiguration: &[]*awsec2.SubnetConfiguration{
				{
					SubnetType: awsec2.SubnetType_PUBLIC,
					Name:       jsii.String("Public"),
					CidrMask:   jsii.Number(24),
				},
				{
					SubnetType: awsec2.SubnetType_PRIVATE,
					Name:       jsii.String("Private"),
					CidrMask:   jsii.Number(24),
				},
			},
			NatGatewayProvider: awsec2.NatInstanceProvider_Gateway(&awsec2.NatGatewayProps{}),
			NatGateways:        jsii.Number(2),
		},
	)

	// create docker image
	strapi := awsecrassets.NewDockerImageAsset(stack, jsii.String("StrapiImage"),
		&awsecrassets.DockerImageAssetProps{
			Directory: jsii.String("./strapi"),
		},
	)

	nginx := awsecrassets.NewDockerImageAsset(stack, jsii.String("NginxImage"),
		&awsecrassets.DockerImageAssetProps{
			Directory: jsii.String("./nginx"),
		},
	)

	// create aurora
	db := awsrds.NewServerlessCluster(stack, jsii.String("StrapiDatabase"),
		&awsrds.ServerlessClusterProps{
			Engine: awsrds.DatabaseClusterEngine_AuroraPostgres(&awsrds.AuroraPostgresClusterEngineProps{
				Version: awsrds.AuroraPostgresEngineVersion_VER_10_12(),
			}),
			DefaultDatabaseName: jsii.String("StrapiDatabase"),
			Scaling: &awsrds.ServerlessScalingOptions{
				AutoPause: awscdk.Duration_Hours(jsii.Number(0)),
			},
			DeletionProtection: jsii.Bool(false),
			BackupRetention:    awscdk.Duration_Days(jsii.Number(7)),
			RemovalPolicy:      awscdk.RemovalPolicy_SNAPSHOT,
			Vpc:                vpc,
		},
	)

	// create efs
	efs := awsefs.NewFileSystem(stack, jsii.String("StrapiFileSystem"),
		&awsefs.FileSystemProps{
			PerformanceMode: awsefs.PerformanceMode_GENERAL_PURPOSE,
			ThroughputMode:  awsefs.ThroughputMode_BURSTING,
			Vpc:             vpc,
		},
	)

	// create fargate volume
	strapiVolume := &awsecs.Volume{
		Name: jsii.String("StrapiVolume"),
		EfsVolumeConfiguration: &awsecs.EfsVolumeConfiguration{
			FileSystemId: efs.FileSystemId(),
		},
	}

	// create fargate task
	task := awsecs.NewFargateTaskDefinition(stack, jsii.String("StrapiDefinition"),
		&awsecs.FargateTaskDefinitionProps{
			Volumes: &[]*awsecs.Volume{
				strapiVolume,
			},
		},
	)

	// add container to task definition
	strapiContainer := task.AddContainer(jsii.String("StrapiContainer"),
		&awsecs.ContainerDefinitionOptions{
			Image: awsecs.ContainerImage_FromDockerImageAsset(strapi),
			Environment: &map[string]*string{
				"DATABASE_HOST": db.ClusterEndpoint().Hostname(),
			},
			Secrets: &map[string]awsecs.Secret{
				"DATABASE_USERNAME": awsecs.Secret_FromSecretsManager(db.Secret(), jsii.String("username")),
				"DATABASE_PASSWORD": awsecs.Secret_FromSecretsManager(db.Secret(), jsii.String("password")),
			},
			PortMappings: &[]*awsecs.PortMapping{
				{ContainerPort: jsii.Number(1337)},
			},
		},
	)
	// strapi mount point = /srv/app
	strapiContainer.AddMountPoints(&awsecs.MountPoint{
		ReadOnly:      jsii.Bool(false),
		ContainerPath: jsii.String("/srv/app"),
		SourceVolume:  strapiVolume.Name,
	})

	nginxContainer := task.AddContainer(jsii.String("NginxContainer"),
		&awsecs.ContainerDefinitionOptions{
			Image: awsecs.AssetImage_FromDockerImageAsset(nginx),
			PortMappings: &[]*awsecs.PortMapping{
				{ContainerPort: jsii.Number(80)},
			},
		},
	)
	nginxContainer.AddMountPoints(&awsecs.MountPoint{
		ReadOnly:      jsii.Bool(true),
		ContainerPath: jsii.String("/srv/app"),
		SourceVolume:  strapiVolume.Name,
	})

	// create fargate
	fargate := ecspatterns.NewApplicationLoadBalancedFargateService(stack, jsii.String("StrapiService"),
		&ecspatterns.ApplicationLoadBalancedFargateServiceProps{
			// TODO: Domain
			TaskDefinition:     task,
			Vpc:                vpc,
			ListenerPort:       jsii.Number(80),
			TargetProtocol:     awselasticloadbalancingv2.ApplicationProtocol_HTTP,
			PublicLoadBalancer: jsii.Bool(true),
		},
	)

	// allow strapi
	db.Connections().AllowDefaultPortFrom(fargate.Service(), jsii.String("Allows Strapi to access Aurora"))
	efs.Connections().AllowDefaultPortFrom(fargate.Service(), jsii.String("Allows Strapi to access EFS"))

	awscdk.NewCfnOutput(stack, jsii.String("LoadBalancerDnsName"), &awscdk.CfnOutputProps{
		Value: fargate.LoadBalancer().LoadBalancerDnsName(),
	})

	return stack
}
