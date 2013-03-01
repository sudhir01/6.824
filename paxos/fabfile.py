from __future__ import with_statement
from fabric.api import run, env, local
from fabric.context_managers import cd
from fabric.contrib.console import confirm

env.hosts = ['athena.dialup.mit.edu']
env.user = 'dbenhaim'

def commit_and_test():
    local("git add -p && git commit")

def test():
    local("ssh dbenhaim@athena.dialup.mit.edu \"cd 6.824/src/ && export GOPATH=$HOME/6.824 && git pull && cd paxos && go test\"")