%define _rpmfilename %%{name}-%%{version}-%%{release}.@ARCHITECTURE@.rpm

Summary: MongoDB Database Tools
Name: mongodb-database-tools
Version: @TOOLS_VERSION@
Release: @TOOLS_RELEASE@
Group: Applications/Databases
License: Apache
URL:        http://www.mongodb.com
Vendor:     MongoDB
BuildArchitectures: @ARCHITECTURE@
Obsoletes:  mongodb-database-tools <= @TOOLS_VERSION@
BuildRoot: %{_topdir}/BUILD/%{name}-%{version}-%{release}

%description
mongodb-database-tools package provides tools for working with the MongoDB server:
 *bsondump - display BSON files in a human-readable format
 *mongoimport - Convert data from JSON, TSV or CSV and insert them into a collection
 *mongoexport - Write an existing collection to CSV or JSON format
 *mongodump/mongorestore - Dump MongoDB backups to disk in .BSON format, 
    or restore them to a live database
 *mongostat - Monitor live MongoDB servers, replica sets, or sharded clusters
 *mongofiles - Read, write, delete, or update files in GridFS 
    (see: http://docs.mongodb.org/manual/core/gridfs/)
 *mongotop - Monitor read/write activity on a mongo server

%changelog
* Wed Feb 12 2020 Patrick Meredith <patrick.meredith@mongodb.com> 50.0.0
- Initial RPM release

%prep
exit 0

%build
echo ${_sourcedir}

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}
cp -a %{_sourcedir}/* %{buildroot}
cd %{buildroot}
tar -xvzf mongodb-database-tools.tar.gz
rm mongodb-database-tools.tar.gz
mv mongodb-database-tools/* ./
rm -Rf mongodb-database-tools

%clean
rm -rf %{buildroot}

%files
%attr(0755,mongod,mongod) /usr/bin/bsondump
%attr(0755,mongod,mongod) /usr/bin/mongodump
%attr(0755,mongod,mongod) /usr/bin/mongoexport
%attr(0755,mongod,mongod) /usr/bin/mongofiles
%attr(0755,mongod,mongod) /usr/bin/mongoimport
%attr(0755,mongod,mongod) /usr/bin/mongorestore
%attr(0755,mongod,mongod) /usr/bin/mongostat
%attr(0755,mongod,mongod) /usr/bin/mongotop
%attr(0644,mongod,mongod) /usr/share/doc/mongodb-database-tools/LICENSE.md
%attr(0644,mongod,mongod) /usr/share/doc/mongodb-database-tools/README.md
%attr(0644,mongod,mongod) /usr/share/doc/mongodb-database-tools/THIRD-PARTY-NOTICES

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
if [ -d /usr/doc -a ! -e /usr/doc/mongodb-database-tools -a -d /usr/share/doc/mongodb-database-tools ]; then
  ln -sf ../share/doc/mongodb-database-tools /usr/doc/mongodb-database-tools
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
   rm -f /usr/share/doc/mongodb-database-tools/*
   # remove the symlink too.
   rm /usr/doc/mongodb-database-tools
fi

exit 0
