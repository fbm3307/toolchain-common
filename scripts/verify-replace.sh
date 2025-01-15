#!/bin/bash
TMP_DIR=/tmp/
BASE_REPO_PATH=$(mktemp -d ${TMP_DIR}replace-verify.XXX)
GH_BASE_URL_KS=https://github.com/kubesaw/
GH_BASE_URL_CRT=https://github.com/codeready-toolchain/
declare -a REPOS=("${GH_BASE_URL_KS}ksctl" "${GH_BASE_URL_CRT}host-operator" "${GH_BASE_URL_CRT}member-operator" "${GH_BASE_URL_CRT}registration-service" "${GH_BASE_URL_CRT}toolchain-e2e")
C_PATH=${PWD}
ERRORREPOLIST=()
ERRORFILELIST=()


echo Initiating verify-replace on dependent repos
for repo in "${REPOS[@]}"
do
    echo =========================================================================================
    echo  
    echo                        "$(basename ${repo})"
    echo                                                                     
    echo =========================================================================================                                            
    repo_path=${BASE_REPO_PATH}/$(basename ${repo})
    ERRFILE=$(mktemp ${TMP_DIR}$(basename ${repo}).XXX)
    echo "Cloning repo in /tmp"
    git clone --depth=1 ${repo} ${repo_path}
    echo "Repo cloned successfully"
    cd ${repo_path}
    if ! make pre-verify; then
        ERRORREPOLIST+="($(basename ${repo}))"
        continue
    fi
    echo "Initiating 'go mod replace' of current toolchain common version in dependent repos"
    go mod edit -replace github.com/codeready-toolchain/toolchain-common=${C_PATH}
    make verify-dependencies 2>&1 | grep -v 'error' ${ERRFILE} 
    rc=$?
    if [ ${rc} -ne 0 ]; then
    ERRORREPOLIST+="($(basename ${repo}))" 
    ERRORFILELIST+="${ERRFILE}  "
    fi
    echo                                                          
    echo =========================================================================================
    echo                                                           
done
echo "Summary"
if [ ${#ERRORREPOLIST[@]} -ne 0 ]; then
    echo "Below are the repos with error: "
    for e in ${ERRORREPOLIST[*]}
    do
        echo "${e}"
    done
    for c in ${ERRORFILELIST[*]}
    do
        echo "${c} has the following errors "
        cat "${c}" 
    done
    exit 1
else
    echo "No errors detected"
fi