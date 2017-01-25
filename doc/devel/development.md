# Developer guide 
This document explains how to setup your dev environment. 

## Prerequisite 
- kubernetes 1.4 
- GO 1.7+
- docker 
- glide 

## Setup
### Step 1
Git Clone to appropriate directory
```
mkdir -p $GOPATH/src/github.com/coreos
cd $GOPATH/src/github/coreos
git clone https://github.com/coreos/etcd-operator.git
cd etcd-operator/
```
### Step 2
Setup your remote repo and create a branch for the fix you are working on
```
git remote add myrepo https://github.com/<GITHUB-USER-NAME>/etcd-operator.git
git checkout -b <FIX-NAME>
```
### Step 3
Export dependencies, make sure you have git and mercurial installed
```
glide install
```
### Step 4
Update the code, unittest and build
```
./hack/build/operator/local_build
```
### Step 5
Commit the change, Push the code and submit a PR
```
git push myrepo <FIX-NAME> 
```
### Step 6
TODO: Docker push
### Step 7
TODO: Update the etcd-operator’s deployment
