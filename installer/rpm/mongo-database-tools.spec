Summary: MongoDB Database Tools
Name: mongodb-database-tools
Version: @TOOLS_VERSION@
Release: 2%{?dist}
Group: Applications/Databases
License: Apache
URL:        http://mms.mongodb.com
Vendor:     MongoDB
BuildArchitectures: @ARCHITECTURE@
Obsoletes:  mongodb-database-tools

%description
mongodb-database-tools package provides tools for working with the MongoDB server:
 *bsondump - display BSON files in a human-readable format
 *mongoimport - Convert data from JSON, TSV or CSV and insert them into a collection
 *mongoexport - Write an existing collection to CSV or JSON format
 *mongodump/mongorestore - Dump MongoDB backups to disk in .BSON format, 
    or restore them to a live database
 *mongostate - Monitor live MongoDB servers, replica sets, or sharded clusters
 *mongofiles - Read, write, delete, or update files in GridFS 
    (see: http://docs.mongodb.org/manual/core/gridfs/)
 *mongotop - Monitor read/write activity on a mongo server

%changelog
* Wed Feb 12 2020 Patrick Meredith <patrick.meredith@mongodb.com> 50.0.0
- Initial RPM release

%prep
echo %{version}

%build
echo ${_sourcedir}

%install
rm -rf $RPM_BUILD_ROOT
mkdir -p $RPM_BUILD_ROOT
cp -a %{_sourcedir}/* $RPM_BUILD_ROOT
if [ -d /usr/doc -a ! -e /usr/doc/mongo-database-tools -a -d /usr/share/doc/mongo-database-tools ]; then
  ln -sf ../share/doc/mongo-database-tools /usr/doc/mongo-database-tools
fi

%clean
rm -rf $RPM_BUILD_ROOT

%files
%attr(0755,mongod,mongod) /usr/bin/bsondump
%attr(0755,mongod,mongod) /usr/bin/bsondump
%attr(0755,mongod,mongod) /usr/bin/mongodump
%attr(0755,mongod,mongod) /usr/bin/mongoexport
%attr(0755,mongod,mongod) /usr/bin/mongofiles
%attr(0755,mongod,mongod) /usr/bin/mongoimport
%attr(0755,mongod,mongod) /usr/bin/mongorestore
%attr(0755,mongod,mongod) /usr/bin/mongostat
%attr(0755,mongod,mongod) /usr/bin/mongotop
%attr(0755,mongod,mongod) /usr/share/doc/mongo-database-tools/LICENSE.md
%attr(0755,mongod,mongod) /usr/share/doc/mongo-database-tools/README.md
%attr(0755,mongod,mongod) /usr/share/doc/mongo-database-tools/THIRD-PARTY-NOTICES
%doc licenses

%pre
# On install
if test $1 = 1; then
    if ! /usr/bin/id -g mongod &>/dev/null; then
        /usr/sbin/groupadd -r mongod
    fi
    if ! /usr/bin/id mongod &>/dev/null; then
        /usr/sbin/useradd -M -r -g mongod -d /var/lib/mongo -s /bin/false -c mongod mongod > /dev/null 2>&1
    fi
fi

exit 0

%post
ln -sf ../versions/mongodb-mms-automation-agent-@AGENT_VERSION@ /opt/mongodb-mms-automation/bin/mongodb-mms-automation-agent
chown -h mongod:mongod /opt/mongodb-mms-automation/bin/mongodb-mms-automation-agent

# On install
if test $1 = 1; then
    /sbin/chkconfig --add mongodb-mms-automation-agent

    # Create empty properties file to support server pool functionality
    if [ "@AGENT_ENV@" = 'hosted' ]; then
        touch /etc/mongodb-mms/server-pool.properties
        chmod 644 /etc/mongodb-mms/server-pool.properties
        chown mongod:mongod /etc/mongodb-mms/server-pool.properties
    fi
fi

# On upgrade, `pre` stopped the service, so start it up again
if test $1 = 2; then
    /etc/init.d/mongodb-mms-automation-agent start
fi

exit 0

%postun
if test $1 = 0; then
   rm -f /usr/bin/bsondump
   rm -f /usr/bin/bsondump
   rm -f /usr/bin/mongodump
   rm -f /usr/bin/mongoexport
   rm -f /usr/bin/mongofiles
   rm -f /usr/bin/mongoimport
   rm -f /usr/bin/mongorestore
   rm -f /usr/bin/mongostat
   rm -f /usr/bin/mongotop
   rm -f /usr/share/doc/mongo-database-tools/*
fi

exit 0
