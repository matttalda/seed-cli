/*
Seed implements a command line interface library to build and run
docker images defined by a seed.manifest.json file.
usage is as folllows:
	seed build [OPTIONS]
		Options:
		-d, -directory	The directory containing the seed spec and Dockerfile
										(default is current directory)

	seed init [OPTIONS]
		Options:
		-d, -directory	The directory to create example seed.manifest.json within
										(default is current directory)

	seed list [OPTIONS]
		Not yet implemented

	seed publish [OPTIONS]
		Not yet implemented

	seed run [OPTIONS]
		Options:
		-i, -inputs  The input data. May be multiple -id flags defined
										(seedfile: Job.Interface.Inputs.Files)
		-in, -imageName The name of the Docker image to run (overrides image name
										pulled from seed spec)
		-o, -outDir			The job output directory. Output defined in
										seedfile: Job.Interface.Outputs.Files and
										Job.Interface.Outputs.Json will be stored relative to
										this directory.
		-s, -schema     The Seed Metadata Schema file; Overrides built in schema to validate
									side-car metadata files against

		-rm				Automatically remove the container when it exits (same as
										docker run --rm)
	seed search [OPTIONS]
		Options:
			-r, -registry	The registry to search

			-o, -org		Limit results to this organization/user (for docker hub, this arg is required as images cannot be listed except by org

			-u, -user		Optional username to use for authentication

			-p, -password	Optional password to use for authentication

	seed validate [OPTIONS]
		Options:
			-d, -directory	The directory containing the seed spec
											(default is current directory)
			-s, -schema			Seed Schema file; Overrides built in schema to validate
											spec against.

	seed version
*/
package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"fmt"
	"strconv"

	"github.com/ngageoint/seed-cli/assets"
	"github.com/ngageoint/seed-cli/commands"
	"github.com/ngageoint/seed-cli/constants"
	"github.com/ngageoint/seed-common/objects"
	"github.com/ngageoint/seed-common/util"
	"github.com/zyxar/image2ascii/ascii"
)

var batchCmd *flag.FlagSet
var buildCmd *flag.FlagSet
var initCmd *flag.FlagSet
var listCmd *flag.FlagSet
var publishCmd *flag.FlagSet
var unpublishCmd *flag.FlagSet
var pullCmd *flag.FlagSet
var runCmd *flag.FlagSet
var searchCmd *flag.FlagSet
var validateCmd *flag.FlagSet
var versionCmd *flag.FlagSet
var specCmd *flag.FlagSet
var cliVersion string

func main() {
	util.InitPrinter(util.PrintErr, os.Stderr, os.Stderr)
	// Handles any panics/actual exits. Ensures deferred functions are called
	// before program exit.
	defer util.HandleExit()

	// Parse input flags
	DefineFlags()

	// seed init: Create example seed.manifest.json. Does not require docker
	if initCmd.Parsed() {
		dir := initCmd.Lookup(constants.JobDirectoryFlag).Value.String()
		version := initCmd.Lookup(constants.VersionFlag).Value.String()
		err := commands.SeedInit(dir, version)
		if err != nil {
			util.PrintUtil("%s\n", err.Error())
			panic(util.Exit{1})
		}
		panic(util.Exit{0})
	}

	// seed validate: Validate seed.manifest.json. Does not require docker
	if validateCmd.Parsed() {
		warningFlag := validateCmd.Lookup(constants.WarnAsErrorsFlag).Value.String() == constants.TrueString
		schemaFile := validateCmd.Lookup(constants.SchemaFlag).Value.String()
		dir := validateCmd.Lookup(constants.JobDirectoryFlag).Value.String()
		version := validateCmd.Lookup(constants.VersionFlag).Value.String()
		err := commands.Validate(warningFlag, schemaFile, dir, version)
		if err != nil {
			util.PrintUtil("%s\n", err.Error())
			panic(util.Exit{1})
		}
		panic(util.Exit{0})
	}

	// seed search: Searches registry for seed images. Does not require docker
	if searchCmd.Parsed() {
		url := searchCmd.Lookup(constants.RegistryFlag).Value.String()
		org := searchCmd.Lookup(constants.OrgFlag).Value.String()
		filter := searchCmd.Lookup(constants.FilterFlag).Value.String()
		username := searchCmd.Lookup(constants.UserFlag).Value.String()
		password := searchCmd.Lookup(constants.PassFlag).Value.String()
		results, err := commands.DockerSearch(url, org, filter, username, password)
		if err != nil {
			util.PrintUtil("%s\n", err.Error())
			panic(util.Exit{1})
		}

		if len(results) > 0 {
			util.PrintUtil("Found %v Repositories:\n", len(results))
			for _, r := range results {
				util.PrintUtil("%s\n", r)
			}
		} else {
			util.PrintUtil("No repositories found.\n")
		}
		panic(util.Exit{0})
	}

	// seed unpublish: Removes an image frome a remote registry
	if unpublishCmd.Parsed() {
		registry := unpublishCmd.Lookup(constants.RegistryFlag).Value.String()
		org := unpublishCmd.Lookup(constants.OrgFlag).Value.String()
		user := unpublishCmd.Lookup(constants.UserFlag).Value.String()
		pass := unpublishCmd.Lookup(constants.PassFlag).Value.String()
		image := unpublishCmd.Lookup(constants.ImgNameFlag).Value.String()
		manifest := unpublishCmd.Lookup(constants.ManifestFlag).Value.String()

		err := commands.DockerUnpublish(image, manifest, registry, org, user, pass)
		if err != nil {
			util.PrintUtil("%s\n", err.Error())
			panic(util.Exit{1})
		}
		panic(util.Exit{0})
	}

	// Checks if we have elevated privileges to run docker. Only required for windows. Prints error message if privileges are missing.
	if runtime.GOOS == "windows" {
		util.CheckSudo()
	}

	// seed list: Lists all seed compliant images on (default) local machine
	if listCmd.Parsed() {
		_, err := commands.DockerList()
		if err != nil {
			util.PrintUtil("%s\n", err.Error())
			panic(util.Exit{1})
		}
		panic(util.Exit{0})
	}

	// seed build: Build Docker image
	if buildCmd.Parsed() {
		warningFlag := buildCmd.Lookup(constants.WarnAsErrorsFlag).Value.String() == constants.TrueString
		jobDirectory := buildCmd.Lookup(constants.JobDirectoryFlag).Value.String()
		version := buildCmd.Lookup(constants.VersionFlag).Value.String()
		user := buildCmd.Lookup(constants.UserFlag).Value.String()
		pass := buildCmd.Lookup(constants.PassFlag).Value.String()
		manifest := buildCmd.Lookup(constants.ManifestFlag).Value.String()
		dockerfile := buildCmd.Lookup(constants.DockerfileFlag).Value.String()
		cacheFrom := buildCmd.Lookup(constants.CacheFromFlag).Value.String()

		imgName, err := commands.DockerBuild(jobDirectory, version, user, pass, manifest, dockerfile, cacheFrom, warningFlag)
		if err != nil {
			util.PrintUtil("%s\n", err.Error())
			panic(util.Exit{1})
		}

		if buildCmd.Lookup(constants.PublishCommand).Value.String() == constants.TrueString {
			registry := buildCmd.Lookup(constants.RegistryFlag).Value.String()
			org := buildCmd.Lookup(constants.OrgFlag).Value.String()
			force := buildCmd.Lookup(constants.ForcePublishFlag).Value.String() == constants.TrueString

			P := buildCmd.Lookup(constants.PkgVersionMajor).Value.String() == constants.TrueString
			pm := buildCmd.Lookup(constants.PkgVersionMinor).Value.String() == constants.TrueString
			pp := buildCmd.Lookup(constants.PkgVersionPatch).Value.String() == constants.TrueString

			J := buildCmd.Lookup(constants.JobVersionMajor).Value.String() == constants.TrueString
			jm := buildCmd.Lookup(constants.JobVersionMinor).Value.String() == constants.TrueString
			jp := buildCmd.Lookup(constants.JobVersionPatch).Value.String() == constants.TrueString

			_, err := commands.DockerPublish(imgName, manifest, registry, org, user, pass, jobDirectory,
				force, P, pm, pp, J, jm, jp)
			if err != nil {
				util.PrintUtil("%s\n", err.Error())
				panic(util.Exit{1})
			}
		}
		panic(util.Exit{0})
	}

	// seed batch: Run Docker image on all files in directory
	if batchCmd.Parsed() {
		batchDir := batchCmd.Lookup(constants.JobDirectoryFlag).Value.String()
		batchFile := batchCmd.Lookup(constants.BatchFlag).Value.String()
		imageName := batchCmd.Lookup(constants.ImgNameFlag).Value.String()
		manifest := batchCmd.Lookup(constants.ManifestFlag).Value.String()
		settings := strings.Split(batchCmd.Lookup(constants.SettingFlag).Value.String(), ",")
		mounts := strings.Split(batchCmd.Lookup(constants.MountFlag).Value.String(), ",")
		outputDir := batchCmd.Lookup(constants.JobOutputDirFlag).Value.String()
		rmFlag := batchCmd.Lookup(constants.RmFlag).Value.String() == constants.TrueString
		metadataSchema := batchCmd.Lookup(constants.SchemaFlag).Value.String()
		err := commands.BatchRun(batchDir, batchFile, imageName, manifest, outputDir, metadataSchema, settings, mounts, rmFlag)
		if err != nil {
			util.PrintUtil("%s\n", err.Error())
			panic(util.Exit{1})
		}
		panic(util.Exit{0})
	}

	// seed run: Runs docker image provided or found in seed manifest
	if runCmd.Parsed() {
		imageName := runCmd.Lookup(constants.ImgNameFlag).Value.String()
		manifest := runCmd.Lookup(constants.ManifestFlag).Value.String()
		inputs := strings.Split(runCmd.Lookup(constants.InputsFlag).Value.String(), ",")
		json := strings.Split(runCmd.Lookup(constants.JsonFlag).Value.String(), ",")
		settings := strings.Split(runCmd.Lookup(constants.SettingFlag).Value.String(), ",")
		mounts := strings.Split(runCmd.Lookup(constants.MountFlag).Value.String(), ",")
		outputDir := runCmd.Lookup(constants.JobOutputDirFlag).Value.String()
		rmFlag := runCmd.Lookup(constants.RmFlag).Value.String() == constants.TrueString
		quiet := runCmd.Lookup(constants.QuietFlag).Value.String() == constants.TrueString
		metadataSchema := runCmd.Lookup(constants.SchemaFlag).Value.String()

		repeat := runCmd.Lookup(constants.RepeatFlag).Value.String()
		reps, err := strconv.Atoi(repeat)
		if err != nil {
			util.PrintUtil("Error reading repeat flag: %s\n", err.Error())
			panic(util.Exit{1})
		}

		// run for any additional repetitions
		if reps > 1 {
			for i := 0; i < reps; i++ {
				outputDirRep := outputDir
				if outputDir != "" {
					outputDirRep = outputDir + fmt.Sprintf("-%d", i)
				}
				_, err := commands.DockerRun(imageName, manifest, outputDirRep, metadataSchema, inputs, json, settings, mounts, rmFlag, quiet)
				if err != nil {
					util.PrintUtil("%s\n", err.Error())
					panic(util.Exit{1})
				}
			}
		} else {
			// run once
			_, err = commands.DockerRun(imageName, manifest, outputDir, metadataSchema, inputs, json, settings, mounts, rmFlag, quiet)
			if err != nil {
				util.PrintUtil("%s\n", err.Error())
				panic(util.Exit{1})
			}
		}
		panic(util.Exit{0})
	}

	// seed publish: Publishes a seed compliant image
	if publishCmd.Parsed() {
		registry := publishCmd.Lookup(constants.RegistryFlag).Value.String()
		org := publishCmd.Lookup(constants.OrgFlag).Value.String()
		user := publishCmd.Lookup(constants.UserFlag).Value.String()
		pass := publishCmd.Lookup(constants.PassFlag).Value.String()
		origImg := publishCmd.Lookup(constants.ImgNameFlag).Value.String()
		manifest := publishCmd.Lookup(constants.ManifestFlag).Value.String()
		jobDirectory := publishCmd.Lookup(constants.JobDirectoryFlag).Value.String()
		force := publishCmd.Lookup(constants.ForcePublishFlag).Value.String() == constants.TrueString

		P := publishCmd.Lookup(constants.PkgVersionMajor).Value.String() == constants.TrueString
		pm := publishCmd.Lookup(constants.PkgVersionMinor).Value.String() == constants.TrueString
		pp := publishCmd.Lookup(constants.PkgVersionPatch).Value.String() == constants.TrueString

		J := publishCmd.Lookup(constants.JobVersionMajor).Value.String() == constants.TrueString
		jm := publishCmd.Lookup(constants.JobVersionMinor).Value.String() == constants.TrueString
		jp := publishCmd.Lookup(constants.JobVersionPatch).Value.String() == constants.TrueString

		_, err := commands.DockerPublish(origImg, manifest, registry, org, user, pass, jobDirectory,
			force, P, pm, pp, J, jm, jp)
		if err != nil {
			util.PrintUtil("%s\n", err.Error())
			panic(util.Exit{1})
		}
		panic(util.Exit{0})
	}

	// seed pull: Pulls a remote image and tags it as a local image
	if pullCmd.Parsed() {
		imageName := pullCmd.Lookup(constants.ImgNameFlag).Value.String()
		registry := pullCmd.Lookup(constants.RegistryFlag).Value.String()
		org := pullCmd.Lookup(constants.OrgFlag).Value.String()
		user := pullCmd.Lookup(constants.UserFlag).Value.String()
		pass := pullCmd.Lookup(constants.PassFlag).Value.String()

		err := commands.DockerPull(imageName, registry, org, user, pass)
		if err != nil {
			util.PrintUtil("%s\n", err.Error())
			panic(util.Exit{1})
		}
		panic(util.Exit{0})
	}
}

//DefineBuildFlags defines the flags for the seed build command
func DefineBuildFlags() {
	// build command flags
	buildCmd = flag.NewFlagSet(constants.BuildCommand, flag.ContinueOnError)
	var cacheFrom string
	buildCmd.StringVar(&cacheFrom, constants.CacheFromFlag, "",
		"Image to use as cache source.")
	buildCmd.StringVar(&cacheFrom, constants.ShortCacheFromFlag, "",
		"Image to use as a cache source.")

	var directory string
	buildCmd.StringVar(&directory, constants.JobDirectoryFlag, ".",
		"Build directory for docker and default location of seed spec and Dockerfile (default is current directory).")
	buildCmd.StringVar(&directory, constants.ShortJobDirectoryFlag, ".",
		"Build directory for docker and default location of seed spec and Dockerfile (default is current directory).")

	var manifest string
	buildCmd.StringVar(&manifest, constants.ManifestFlag, ".",
		"Manifest file to use (default is seed.manifest.json in the build directory).")
	buildCmd.StringVar(&manifest, constants.ShortManifestFlag, ".",
		"Manifest file to use (default is seed.manifest.json in the build directory).")

	var dockerfile string
	buildCmd.StringVar(&dockerfile, constants.DockerfileFlag, ".",
		"Dockerfile to use (default is <build directory>/Dockerfile);")
	buildCmd.StringVar(&dockerfile, constants.ShortDockerfileFlag, ".",
		"Dockerfile to use (default is <build directory>/Dockerfile);")

	var version string
	buildCmd.StringVar(&version, constants.VersionFlag, "1.0.0",
		"Version of example seed manifest to use (default is 1.0.0).")
	buildCmd.StringVar(&version, constants.ShortVersionFlag, "1.0.0",
		"Version of example seed manifest to use (default is 1.0.0).")

	var user string
	buildCmd.StringVar(&user, constants.UserFlag, "",
		"Optional username to use if dockerfile pulls images from private repository (default is anonymous).")
	buildCmd.StringVar(&user, constants.ShortUserFlag, "",
		"Optional username to use if dockerfile pulls images from private repository (default is anonymous).")

	var password string
	buildCmd.StringVar(&password, constants.PassFlag, "",
		"Optional password if dockerfile pulls images from private repository (default is empty).")
	buildCmd.StringVar(&password, constants.ShortPassFlag, "",
		"Optional password if dockerfile pulls images from private repository (default is empty).")

	var warningsAsErrors bool
	buildCmd.BoolVar(&warningsAsErrors, constants.WarnAsErrorsFlag, false,
		"Treats validation warnings as errors")
	buildCmd.BoolVar(&warningsAsErrors, constants.ShortWarnAsErrorsFlag, false,
		"Treats validation warnings as errors")

	var publish bool
	buildCmd.BoolVar(&publish, constants.PublishCommand, false, "Publishes image after successful build.")

	var registry string
	buildCmd.StringVar(&registry, constants.RegistryFlag, "", "Specifies registry to publish image to.")
	buildCmd.StringVar(&registry, constants.ShortRegistryFlag, "", "Specifies registry to publish image to.")

	var org string
	buildCmd.StringVar(&org, constants.OrgFlag, "", "Specifies organization to publish image to.")
	buildCmd.StringVar(&org, constants.ShortOrgFlag, "", "Specifies organization to publish image to.")

	var b bool
	buildCmd.BoolVar(&b, constants.ForcePublishFlag, false,
		"Force publish, do not deconflict")
	var pPatch bool
	buildCmd.BoolVar(&pPatch, constants.PkgVersionPatch, false,
		"Patch version bump of 'packageVersion' in manifest on disk, will auto rebuild and push")
	var pMin bool
	buildCmd.BoolVar(&pMin, constants.PkgVersionMinor, false,
		"Minor version bump of 'packageVersion' in manifest on disk, will auto rebuild and push")
	var pMaj bool
	buildCmd.BoolVar(&pMaj, constants.PkgVersionMajor, false,
		"Major version bump of 'packageVersion' in manifest on disk, will auto rebuild and push")
	var jPatch bool
	buildCmd.BoolVar(&jPatch, constants.JobVersionPatch, false,
		"Patch version bump of 'jobVersion' in manifest on disk, will auto rebuild and push")
	var jMin bool
	buildCmd.BoolVar(&jMin, constants.JobVersionMinor, false,
		"Minor version bump of 'jobVersion' in manifest on disk, will auto rebuild and push")
	var jMaj bool
	buildCmd.BoolVar(&jMaj, constants.JobVersionMajor, false,
		"Major version bump of 'jobVersion' in manifest on disk, will auto rebuild and push")

	// Print usage function
	buildCmd.Usage = func() {
		PrintASCIIArt()
		commands.PrintBuildUsage()
	}
}

//DefineInitFlags defines the flags for the seed init command
func DefineInitFlags() {
	// build command flags
	initCmd = flag.NewFlagSet(constants.InitCommand, flag.ContinueOnError)
	var directory string
	initCmd.StringVar(&directory, constants.JobDirectoryFlag, ".",
		"Directory to place example seed.manifest.json (default is current directory).")
	initCmd.StringVar(&directory, constants.ShortJobDirectoryFlag, ".",
		"Directory to place example seed.manifest.json (default is current directory).")
	var version string
	initCmd.StringVar(&version, constants.VersionFlag, "1.0.0",
		"Version of example seed manifest to use (default is 1.0.0).")
	initCmd.StringVar(&version, constants.ShortVersionFlag, "1.0.0",
		"Version of example seed manifest to use (default is 1.0.0).")

	// Print usage function
	initCmd.Usage = func() {
		PrintASCIIArt()
		commands.PrintInitUsage()
	}
}

//DefineRunFlags defines the flags for the seed run command
func DefineBatchFlags() {
	batchCmd = flag.NewFlagSet(constants.BatchCommand, flag.ContinueOnError)

	var directory string
	batchCmd.StringVar(&directory, constants.JobDirectoryFlag, ".",
		"Directory of files to batch process (default is current directory)")
	batchCmd.StringVar(&directory, constants.ShortJobDirectoryFlag, ".",
		"Directory of files to batch process (default is current directory)")

	var batchFile string
	batchCmd.StringVar(&batchFile, constants.BatchFlag, "",
		"File specifying input keys and file mapping for batch processing")
	batchCmd.StringVar(&batchFile, constants.ShortBatchFlag, "",
		"File specifying input keys and file mapping for batch processing")

	var imgNameFlag string
	batchCmd.StringVar(&imgNameFlag, constants.ImgNameFlag, "",
		"Name of Docker image to run")
	batchCmd.StringVar(&imgNameFlag, constants.ShortImgNameFlag, "",
		"Name of Docker image to run")

	var manifest string
	batchCmd.StringVar(&manifest, constants.ManifestFlag, ".",
		"Manifest file to use (default is seed.manifest.json in the current directory).")
	batchCmd.StringVar(&manifest, constants.ShortManifestFlag, ".",
		"Manifest file to use (default is seed.manifest.json in the current directory).")

	var settings objects.ArrayFlags
	batchCmd.Var(&settings, constants.SettingFlag,
		"Defines the value to be applied to setting")
	batchCmd.Var(&settings, constants.ShortSettingFlag,
		"Defines the value to be applied to setting")

	var mounts objects.ArrayFlags
	batchCmd.Var(&mounts, constants.MountFlag,
		"Defines the full path to be mapped via mount")
	batchCmd.Var(&mounts, constants.ShortMountFlag,
		"Defines the full path to be mapped via mount")

	var outdir string
	batchCmd.StringVar(&outdir, constants.JobOutputDirFlag, "",
		"Full path to the job output directory")
	batchCmd.StringVar(&outdir, constants.ShortJobOutputDirFlag, "",
		"Full path to the job output directory")

	var rmVar bool
	batchCmd.BoolVar(&rmVar, constants.RmFlag, false,
		"Specifying the -rm flag automatically removes the image after executing docker run")

	var metadataSchema string
	batchCmd.StringVar(&metadataSchema, constants.SchemaFlag, "",
		"Metadata schema file to override built in schema in validating side-car metadata files")
	batchCmd.StringVar(&metadataSchema, constants.ShortSchemaFlag, "",
		"Metadata schema file to override built in schema in validating side-car metadata files")

	// Run usage function
	batchCmd.Usage = func() {
		PrintASCIIArt()
		commands.PrintBatchUsage()
	}
}

//DefineRunFlags defines the flags for the seed run command
func DefineRunFlags() {
	runCmd = flag.NewFlagSet(constants.RunCommand, flag.ContinueOnError)

	var imgNameFlag string
	runCmd.StringVar(&imgNameFlag, constants.ImgNameFlag, "",
		"Name of Docker image to run")
	runCmd.StringVar(&imgNameFlag, constants.ShortImgNameFlag, "",
		"Name of Docker image to run")

	var manifest string
	runCmd.StringVar(&manifest, constants.ManifestFlag, ".",
		"Manifest file to use (default is seed.manifest.json in the current directory).")
	runCmd.StringVar(&manifest, constants.ShortManifestFlag, ".",
		"Manifest file to use (default is seed.manifest.json in the current directory).")

	var inputs objects.ArrayFlags
	runCmd.Var(&inputs, constants.InputsFlag,
		"Defines the full path to any input data arguments")
	runCmd.Var(&inputs, constants.ShortInputsFlag,
		"Defines the full path to input data arguments")

	var json objects.ArrayFlags
	runCmd.Var(&json, constants.JsonFlag,
		"Defines input json arguments")
	runCmd.Var(&json, constants.ShortJsonFlag,
		"Defines input json arguments")

	var settings objects.ArrayFlags
	runCmd.Var(&settings, constants.SettingFlag,
		"Defines the value to be applied to setting")
	runCmd.Var(&settings, constants.ShortSettingFlag,
		"Defines the value to be applied to setting")

	var mounts objects.ArrayFlags
	runCmd.Var(&mounts, constants.MountFlag,
		"Defines the full path to be mapped via mount")
	runCmd.Var(&mounts, constants.ShortMountFlag,
		"Defines the full path to be mapped via mount")

	var outdir string
	runCmd.StringVar(&outdir, constants.JobOutputDirFlag, "",
		"Full path to the job output directory")
	runCmd.StringVar(&outdir, constants.ShortJobOutputDirFlag, "",
		"Full path to the job output directory")

	var rmVar bool
	runCmd.BoolVar(&rmVar, constants.RmFlag, false,
		"Specifying the -rm flag automatically removes the image after executing docker run")

	var quiet bool
	runCmd.BoolVar(&quiet, constants.QuietFlag, false,
		"Specifying the -q flag disables output from the docker image being run")
	runCmd.BoolVar(&quiet, constants.ShortQuietFlag, false,
		"Specifying the -q flag disables output from the docker image being run")

	var metadataSchema string
	runCmd.StringVar(&metadataSchema, constants.SchemaFlag, "",
		"Metadata schema file to override built in schema in validating side-car metadata files")
	runCmd.StringVar(&metadataSchema, constants.ShortSchemaFlag, "",
		"Metadata schema file to override built in schema in validating side-car metadata files")

	var repeat int
	runCmd.IntVar(&repeat, constants.RepeatFlag, 1,
		"Run the docker image the specified number of times")
	runCmd.IntVar(&repeat, constants.ShortRepeatFlag, 1,
		"Run the docker image the specified number of times")

	// Run usage function
	runCmd.Usage = func() {
		PrintASCIIArt()
		commands.PrintRunUsage()
	}
}

//DefineListFlags defines the flags for the seed list command
func DefineListFlags() {
	listCmd = flag.NewFlagSet("list", flag.ExitOnError)
	listCmd.Usage = func() {
		PrintASCIIArt()
		commands.PrintListUsage()
	}
}

//DefineSearchFlags defines the flags for the seed search command
func DefineSearchFlags() {
	// Search command
	searchCmd = flag.NewFlagSet(constants.SearchCommand, flag.ExitOnError)
	var registry string
	searchCmd.StringVar(&registry, constants.RegistryFlag, "", "Specifies registry to search (default is index.docker.io).")
	searchCmd.StringVar(&registry, constants.ShortRegistryFlag, "", "Specifies registry to search (default is index.docker.io).")

	var org string
	searchCmd.StringVar(&org, constants.OrgFlag, "", "Specifies organization to filter (default is no filter, search all orgs).")
	searchCmd.StringVar(&org, constants.ShortOrgFlag, "", "Specifies organization to filter (default is no filter, search all orgs).")

	var filter string
	searchCmd.StringVar(&filter, constants.FilterFlag, "", "Specifies filter to apply (default is no filter).")
	searchCmd.StringVar(&filter, constants.ShortFilterFlag, "", "Specifies filter to apply (default is no filter).")

	var user string
	searchCmd.StringVar(&user, constants.UserFlag, "", "Specifies username to use for authorization (default is anonymous).")
	searchCmd.StringVar(&user, constants.ShortUserFlag, "", "Specifies username to use for authorization (default is anonymous).")

	var password string
	searchCmd.StringVar(&password, constants.PassFlag, "", "Specifies password to use for authorization (default is empty).")
	searchCmd.StringVar(&password, constants.ShortPassFlag, "", "Specifies password to use for authorization (default is empty).")

	searchCmd.Usage = func() {
		PrintASCIIArt()
		commands.PrintSearchUsage()
	}
}

//DefinePublishFlags defines the flags for the seed publish command
func DefinePublishFlags() {
	publishCmd = flag.NewFlagSet(constants.PublishCommand, flag.ExitOnError)
	var registry string
	publishCmd.StringVar(&registry, constants.RegistryFlag, "", "Specifies registry to publish image to.")
	publishCmd.StringVar(&registry, constants.ShortRegistryFlag, "", "Specifies registry to publish image to.")

	var org string
	publishCmd.StringVar(&org, constants.OrgFlag, "", "Specifies organization to publish image to.")
	publishCmd.StringVar(&org, constants.ShortOrgFlag, "", "Specifies organization to publish image to.")

	var imgNameFlag string
	publishCmd.StringVar(&imgNameFlag, constants.ImgNameFlag, "",
		"Name of Docker image to publish")
	publishCmd.StringVar(&imgNameFlag, constants.ShortImgNameFlag, "",
		"Name of Docker image to publish")

	var manifest string
	publishCmd.StringVar(&manifest, constants.ManifestFlag, ".",
		"Manifest file to use (default is seed.manifest.json in the current directory).")
	publishCmd.StringVar(&manifest, constants.ShortManifestFlag, ".",
		"Manifest file to use (default is seed.manifest.json in the current directory).")

	var d string
	publishCmd.StringVar(&d, constants.JobDirectoryFlag, ".",
		"Directory of seed spec and Dockerfile (default is current directory).")
	publishCmd.StringVar(&d, constants.ShortJobDirectoryFlag, ".",
		"Directory of seed spec and Dockerfile (default is current directory).")

	var b bool
	publishCmd.BoolVar(&b, constants.ForcePublishFlag, false,
		"Force publish, do not deconflict")
	var pPatch bool
	publishCmd.BoolVar(&pPatch, constants.PkgVersionPatch, false,
		"Patch version bump of 'packageVersion' in manifest on disk, will auto rebuild and push")
	var pMin bool
	publishCmd.BoolVar(&pMin, constants.PkgVersionMinor, false,
		"Minor version bump of 'packageVersion' in manifest on disk, will auto rebuild and push")
	var pMaj bool
	publishCmd.BoolVar(&pMaj, constants.PkgVersionMajor, false,
		"Major version bump of 'packageVersion' in manifest on disk, will auto rebuild and push")
	var jPatch bool
	publishCmd.BoolVar(&jPatch, constants.JobVersionPatch, false,
		"Patch version bump of 'jobVersion' in manifest on disk, will auto rebuild and push")
	var jMin bool
	publishCmd.BoolVar(&jMin, constants.JobVersionMinor, false,
		"Minor version bump of 'jobVersion' in manifest on disk, will auto rebuild and push")
	var jMaj bool
	publishCmd.BoolVar(&jMaj, constants.JobVersionMajor, false,
		"Major version bump of 'jobVersion' in manifest on disk, will auto rebuild and push")

	var user string
	publishCmd.StringVar(&user, constants.UserFlag, "", "Specifies username to use for authorization (default is anonymous).")
	publishCmd.StringVar(&user, constants.ShortUserFlag, "", "Specifies username to use for authorization (default is anonymous).")

	var password string
	publishCmd.StringVar(&password, constants.PassFlag, "", "Specifies password to use for authorization (default is empty).")
	publishCmd.StringVar(&password, constants.ShortPassFlag, "", "Specifies password to use for authorization (default is empty).")

	publishCmd.Usage = func() {
		PrintASCIIArt()
		commands.PrintPublishUsage()
	}
}

//DefineUnpublishFlags defines the flags for the seed unpublish command
func DefineUnpublishFlags() {
	unpublishCmd = flag.NewFlagSet(constants.UnpublishCommand, flag.ExitOnError)

	var imgNameFlag string
	unpublishCmd.StringVar(&imgNameFlag, constants.ImgNameFlag, "",
		"Name of Docker image to remove from registry")
	unpublishCmd.StringVar(&imgNameFlag, constants.ShortImgNameFlag, "",
		"Name of Docker image to remove from registry")

	var manifest string
	unpublishCmd.StringVar(&manifest, constants.ManifestFlag, ".",
		"Manifest file to use (default is seed.manifest.json in the current directory).")
	unpublishCmd.StringVar(&manifest, constants.ShortManifestFlag, ".",
		"Manifest file to use (default is seed.manifest.json in the current directory).")

	var registry string
	unpublishCmd.StringVar(&registry, constants.RegistryFlag, "", "Specifies registry to remove image from.")
	unpublishCmd.StringVar(&registry, constants.ShortRegistryFlag, "", "Specifies registry to remove image from.")

	var org string
	unpublishCmd.StringVar(&org, constants.OrgFlag, "", "Specifies organization to remove image from.")
	unpublishCmd.StringVar(&org, constants.ShortOrgFlag, "", "Specifies organization to remove image from.")

	var user string
	unpublishCmd.StringVar(&user, constants.UserFlag, "", "Specifies username to use for authorization (default is anonymous).")
	unpublishCmd.StringVar(&user, constants.ShortUserFlag, "", "Specifies username to use for authorization (default is anonymous).")

	var password string
	unpublishCmd.StringVar(&password, constants.PassFlag, "", "Specifies password to use for authorization (default is empty).")
	unpublishCmd.StringVar(&password, constants.ShortPassFlag, "", "Specifies password to use for authorization (default is empty).")

	unpublishCmd.Usage = func() {
		PrintASCIIArt()
		commands.PrintUnpublishUsage()
	}
}

//DefinePullFlags defines the flags for the seed pull command
func DefinePullFlags() {
	// Search command
	pullCmd = flag.NewFlagSet(constants.PullCommand, flag.ExitOnError)

	var imgNameFlag string
	pullCmd.StringVar(&imgNameFlag, constants.ImgNameFlag, "",
		"Name of Docker image to pull")
	pullCmd.StringVar(&imgNameFlag, constants.ShortImgNameFlag, "",
		"Name of Docker image to pull")

	var registry string
	pullCmd.StringVar(&registry, constants.RegistryFlag, "", "Specifies registry to pull image from (default is index.docker.io).")
	pullCmd.StringVar(&registry, constants.ShortRegistryFlag, "", "Specifies registry to pull image from (default is index.docker.io).")

	var org string
	pullCmd.StringVar(&org, constants.OrgFlag, "", "Specifies organization to pull image from (default is geoint).")
	pullCmd.StringVar(&org, constants.ShortOrgFlag, "", "Specifies organization to pull image from (default is geoint).")

	var user string
	pullCmd.StringVar(&user, constants.UserFlag, "", "Specifies username to use for authorization (default is anonymous).")
	pullCmd.StringVar(&user, constants.ShortUserFlag, "", "Specifies username to use for authorization (default is anonymous).")

	var password string
	pullCmd.StringVar(&password, constants.PassFlag, "", "Specifies password to use for authorization (default is empty).")
	pullCmd.StringVar(&password, constants.ShortPassFlag, "", "Specifies password to use for authorization (default is empty).")

	pullCmd.Usage = func() {
		PrintASCIIArt()
		commands.PrintPullUsage()
	}
}

//DefineValidateFlags defines the flags for the validate command
func DefineValidateFlags() {
	validateCmd = flag.NewFlagSet(constants.ValidateCommand, flag.ExitOnError)
	var warningsAsErrors bool
	validateCmd.BoolVar(&warningsAsErrors, constants.WarnAsErrorsFlag, false,
		"Treats validation warnings as errors")
	validateCmd.BoolVar(&warningsAsErrors, constants.ShortWarnAsErrorsFlag, false,
		"Treats validation warnings as errors")
	var directory string
	validateCmd.StringVar(&directory, constants.JobDirectoryFlag, ".",
		"Location of the seed.manifest.json spec to validate")
	validateCmd.StringVar(&directory, constants.ShortJobDirectoryFlag, ".",
		"Location of the seed.manifest.json spec to validate")
	var schema string
	validateCmd.StringVar(&schema, constants.SchemaFlag, "",
		"JSON schema file to validate seed against.")
	validateCmd.StringVar(&schema, constants.ShortSchemaFlag, "",
		"JSON schema file to validate seed against.")
	var version string
	validateCmd.StringVar(&version, constants.VersionFlag, "1.0.0",
		"Version of example seed manifest to use (default is 1.0.0).")
	validateCmd.StringVar(&version, constants.ShortVersionFlag, "1.0.0",
		"Version of example seed manifest to use (default is 1.0.0).")

	validateCmd.Usage = func() {
		PrintASCIIArt()
		commands.PrintValidateUsage()
	}
}

//DefineFlags defines the flags available for the seed runner.
func DefineFlags() {
	// Seed subcommand flags
	DefineBatchFlags()
	DefineBuildFlags()
	DefineInitFlags()
	DefineRunFlags()
	DefineListFlags()
	DefineSearchFlags()
	DefinePublishFlags()
	DefineUnpublishFlags()
	DefinePullFlags()
	DefineValidateFlags()
	versionCmd = flag.NewFlagSet(constants.VersionCommand, flag.ExitOnError)
	versionCmd.Usage = func() {
		PrintVersionUsage()
	}

	specCmd = flag.NewFlagSet(constants.SpecCommand, flag.ExitOnError)
	specCmd.Usage = func() {
		PrintSpecUsage()
	}

	// Print usage if no command given
	if len(os.Args) == 1 {
		PrintUsage()
		panic((util.Exit{0}))
	}

	var cmd *flag.FlagSet
	minArgs := 2

	// Parse commands
	switch os.Args[1] {

	case constants.BatchCommand:
		cmd = batchCmd
		minArgs = 2

	case constants.BuildCommand:
		cmd = buildCmd

		// Check for seed manifest in current directory. If found, add current directory arg
		if len(os.Args) == 2 {
			if _, exist, err := util.GetSeedFileName("."); err == nil && exist {
				os.Args = append(os.Args, ".")
			}
		}
		minArgs = 3

	case constants.InitCommand:
		cmd = initCmd
		minArgs = 2

	case constants.RunCommand:
		cmd = runCmd
		minArgs = 2

	case constants.SearchCommand:
		cmd = searchCmd
		minArgs = 2

	case constants.ListCommand:
		cmd = listCmd
		minArgs = 2

	case constants.PublishCommand:
		cmd = publishCmd
		minArgs = 2

	case constants.UnpublishCommand:
		cmd = unpublishCmd
		minArgs = 2

	case constants.PullCommand:
		cmd = pullCmd
		minArgs = 3

	case constants.ValidateCommand:
		cmd = validateCmd
		minArgs = 2

	case constants.VersionCommand:
		versionCmd.Parse(os.Args[2:])
		PrintVersion()

	case constants.SpecCommand:
		specCmd.Parse(os.Args[2:])
		PrintSpec()

	default:
		util.PrintUtil("%q is not a valid command.\n", os.Args[1])
		PrintUsage()
		panic(util.Exit{0})
	}

	if cmd != nil {
		err := cmd.Parse(os.Args[2:])
		if err == flag.ErrHelp {
			panic(util.Exit{0})
		}
		if len(os.Args) < minArgs {
			cmd.Usage()
			panic(util.Exit{0})
		}
	}
}

//PrintUsage prints the seed usage arguments
func PrintUsage() {
	PrintASCIIArt()
	util.PrintUtil("\nUsage:\tseed COMMAND\n\n")
	util.PrintUtil("A tool for assisting in creating seed spec compliant algorithms\n\n")
	util.PrintUtil("Commands:\n")
	util.PrintUtil("  build \tBuilds Seed compliant Docker image\n")
	util.PrintUtil("  batch \tExecutes Seed compliant docker image over multiple iterations\n")
	util.PrintUtil("  init  \tInitialize new project with example seed.manifest.json file\n")
	util.PrintUtil("  list  \tLists all Seed compliant images residing on the local system\n")
	util.PrintUtil("  publish\tPublishes Seed compliant images to remote Docker registry\n")
	util.PrintUtil("  pull\t\tPulls images from remote Docker registry\n")
	util.PrintUtil("  run   \tExecutes Seed compliant Docker image\n")
	util.PrintUtil("  search\tAllows for discovery of Seed compliant images hosted within a Docker registry (default is docker.io)\n")
	util.PrintUtil("  spec\t\tDisplays the specification for the current Seed version\n")
	util.PrintUtil("  unpublish\tRemoves images from remote Docker registry\n")
	util.PrintUtil("  validate\tValidates a Seed spec\n")
	util.PrintUtil("  version\tPrints the version of Seed spec\n")
	util.PrintUtil("\nRun 'seed COMMAND --help' for more information on a command.\n")
	panic(util.Exit{0})
}

//PrintVersionUsage prints the seed version usage, then exits the program
func PrintVersionUsage() {
	PrintASCIIArt()
	util.PrintUtil("\nUsage:\tseed version \n")
	util.PrintUtil("\nOutputs the version of the Seed CLI and specification.\n")
	panic(util.Exit{0})
}

//PrintVersion prints the seed CLI version
func PrintVersion() {
	PrintASCIIArt()
	util.PrintUtil("Seed CLI v%s\n", cliVersion)
	schemas, err := assets.AssetDir("schema")
	if err != nil {
		util.PrintUtil("Error getting supported schema versions: %s \n", err.Error())
		panic(util.Exit{1})
	}
	util.PrintUtil("Supported Seed schema versions: %s\n", schemas)
	panic(util.Exit{0})
}

//PrintSpec shows the seed-spec manual page as help text
func PrintSpec() error {
	cmd := exec.Command("man", "seed-spec")
	var errs bytes.Buffer
	if util.StdErr != nil {
		cmd.Stderr = io.MultiWriter(util.StdErr, &errs)
	} else {
		cmd.Stderr = &errs
	}
	cmd.Stdout = util.StdErr
	// Run docker build
	cmd.Run()
	if errs.String() != "" {
		util.PrintUtil("ERROR: Error displaying seed spec:\n%s\n",
			errs.String())
		util.PrintUtil("Exiting seed...\n")
		return errors.New(errs.String())
	}
	return nil
}

//PrintSpecUsage prints the seed build usage arguments, then exits the program
func PrintSpecUsage() {
	util.PrintUtil("\nUsage:\tseed spec\n")
	util.PrintUtil("\nDisplays the manual entry for the seed spec\n")
	return
}

//PrintAsciiArt prints the ascii art before any seed help
func PrintASCIIArt() {
	imgBytes, _ := assets.Asset("images/wordmark.png")
	a, _ := ascii.Decode(bytes.NewReader(imgBytes), ascii.Options{Color: true, Height: 12})
	a.WriteTo(os.Stdout)
}
