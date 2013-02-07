from __future__ import with_statement
from fabric.api import run, env
from fabric.context_managers import cd
from fabric.contrib.console import confirm

env.hosts = ['athena.dialup.mit.edu']
env.user = 'dbenhaim'
env.password = 'Tiger3010'


def commit():
    local("git add -p && git commit")

def push():
    local("git push")

def prepare_deploy():
    commit()
    push()

def deploy():
    code_dir = '/afs/athena.mit.edu/user/d/b/dbenhaim/6.824'
    while cd(code_dir):
        run("cd /afs/athena.mit.edu/user/d/b/dbenhaim/6.824", shell=False)
        run("ls", shell=False)
        run("add ggo_v1.0.1", shell=False)
        run("PATH=/mit/ggo_v1.0.1/bin:$PATH",shell=False)
        run("export GOROOT=/mit/ggo_v1.0.1/go",shell=False)
        run("go test", shell=False)