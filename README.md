# `glance` - kubectl plugin to view cluster resource allocation and usage

Utility to quickly view Kubernetes cluster resources.

`glance` is a kubectl plugin written in go that shows cluster resource allocation and utilization.  It allows you to quickly see the usage and allocation of CPU and memory across a cluster.


## Installation

### krew (kubectl plugin manager)

1. [Install krew](https://github.com/GoogleContainerTools/krew)
   plugin manager for kubectl.
1. Run `kubectl krew install glance`.

### macOS

On macOS, plugin can be installed via [Homebrew](https://brew.sh):

```shell
brew tap davidxarnold/glance
brew install kubectl-glance
```



## Usage 
**`kubectl glance`**.
```shell
INFO[0000] There are 1 node(s) in the cluster            Host="https://kubernetes.docker.internal:6443"
 NODE NAME       STATUS  PROVIDERID  ALLOCATABLE  ALLOCATABLE  ALLOCATED  ALLOCATED  ALLOCATED  ALLOCATED  USAGE        USAGE     
                                     CPU          MEM (MI)     CPU REQ    CPU LIM    MEM REQ    MEM LIM    CPU          MEM       
 node-1                     4            8053040Ki    1.250      0          396Mi      340Mi      0.186332670  1172148Ki 
 TOTALS                              4            8053040KI    1.250      0.000      396MI      340MI      0.186332670  1172148KI 
```


## Build Instructions

```
make build
```