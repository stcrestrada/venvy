package modules

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/pnegahdar/venvy/util"
	"github.com/pnegahdar/venvy/venvy"
	"io/ioutil"
	"path"
	"strings"
	"path/filepath"
)

const DefaultPython = "python"
const DefaultVirtualenv = "virtualenv"
const DefaultPipInstallCommand = "pip install"

type PyModuleConfig struct {
	Python            string
	AutoInstallDeps   bool   `json:"auto_install_deps"`
	Dependencies      []string
	PipInstallCommand string `json:"pip_install_command"`
	VirtualEnvCommand string `json:"virtualenv_command"`
}

type PythonModule struct {
	manager *venvy.ProjectManager
	config  *PyModuleConfig
	name    string
}

func (pm *PythonModule) venvDir() string {
	return pm.manager.StoragePath("pyvenvs", pm.name)
}

// All the activation needed, essentially what venv/bin/activate does
func (pm *PythonModule) venvEnvarModule() *EnvvarModule {
	return &EnvvarModule{config: &EnvVarConfig{
		Vars: map[string]string{
			"VIRTUAL_ENV": pm.venvDir(),
			"PATH":        fmt.Sprintf("%s/bin:${PATH}", pm.venvDir()),
		},
		UnsetVars: []string{"PYTHONHOME"},
	}}
}

func (pm *PythonModule) autoInstallHashPath() string {
	return path.Join(pm.venvDir(), "autoinstall_dep_sha.txt")
}

func (pm *PythonModule) autoInstallLastHash() string {
	data, err := ioutil.ReadFile(pm.autoInstallHashPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (pm *PythonModule) autoInstallCalculateDepHash() (string, error) {
	if len(pm.config.Dependencies) == 0 {
		return "", nil
	}
	hash := md5.New()
	for _, dep := range pm.config.Dependencies {
		// read the deps if its a file, preferring .txt instead of isFile type check for safety sake
		if strings.HasSuffix(dep, ".txt") {
			fullPath := util.MustExpandPath(dep)
			if !path.IsAbs(dep) {
				fullPath = pm.manager.RootPath(dep)
			}
			data, err := ioutil.ReadFile(fullPath)
			if err != nil {
				return "", err
			}
			hash.Write(data)
		} else {
			hash.Write([]byte(dep))
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (pm *PythonModule) autoInstallCmds() []string {
	cmds := []string{}
	for _, dep := range pm.config.Dependencies {
		cmd := pm.config.PipInstallCommand + " "
		if strings.HasSuffix(dep, ".txt") {
			fullPath := pm.manager.RootPath(dep)
			cmd += fmt.Sprintf("-r %s", fullPath)
		} else {
			cmd += dep
		}
		cmds = append(cmds, cmd)
	}
	return cmds
}

func (pm *PythonModule) venvExists() bool {
	return util.PathExists(filepath.Join(pm.venvDir(), "bin"))
}

func (pm *PythonModule) ShellActivateCommands() ([]string, error) {
	currentDepHash, err := pm.autoInstallCalculateDepHash()
	if err != nil {
		return nil, err
	}
	lastDepHash := pm.autoInstallLastHash()
	hashChanged := currentDepHash != lastDepHash
	lines := []string{}
	addArgs := func(args ...string) {
		lines = append(lines, strings.Join(args, " "))
	}
	if !pm.venvExists() {
		// Create the venv [virtualenv -p python /path/to/venv]
		addArgs(pm.config.VirtualEnvCommand, "-p", pm.config.Python, pm.venvDir())
	}

	eVModule := pm.venvEnvarModule()
	evCommands, err := eVModule.ShellActivateCommands()
	if err != nil {
		return nil, err
	}
	lines = append(lines, evCommands...)

	if hashChanged && pm.config.AutoInstallDeps {
		// run the install [pip install -r requirements.txt deps]
		lines = append(lines, pm.autoInstallCmds()...)
		// write the hash so we don't reinstall these deps [echo sd2if1jdfs > .venvy/project/pyvenv/auto_install.txt]
		addArgs("echo", currentDepHash, ">", pm.autoInstallHashPath())
	}
	return lines, nil
}

func (pm *PythonModule) ShellDeactivateCommands() ([]string, error) {
	evModule := pm.venvEnvarModule()
	lines, err := evModule.ShellDeactivateCommands()
	if err != nil {
		return nil, err
	}
	return lines, nil
}

func NewPythonModule(manager *venvy.ProjectManager, self *venvy.Module) (venvy.Moduler, error) {
	moduleConfig := &PyModuleConfig{}
	err := util.UnmarshalEmpty(self.Config, moduleConfig)
	if err != nil {
		return nil, err
	}
	if moduleConfig.Python == "" {
		moduleConfig.Python = DefaultPython
	}
	if moduleConfig.PipInstallCommand == "" {
		moduleConfig.PipInstallCommand = DefaultPipInstallCommand
	}
	if moduleConfig.VirtualEnvCommand == "" {
		moduleConfig.VirtualEnvCommand = DefaultVirtualenv

	}
	return &PythonModule{manager: manager, config: moduleConfig, name: self.Name}, nil
}
