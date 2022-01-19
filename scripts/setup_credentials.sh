set -x
set -v
set -e
cat > mci.buildlogger <<END_OF_CREDS
slavename='${slave}'
passwd='${passwd}'
builder='MCI_${build_variant}'
build_num=${builder_num}
build_phase='${task_name}_${execution}'
END_OF_CREDS
# Resmoke hardcodes the location of this file so we need to copy it to the working directory
# we run resmoke from.
cp mci.buildlogger test/qa-tests