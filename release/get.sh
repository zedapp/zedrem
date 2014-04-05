#!/bin/sh

OS=`uname -s`
PROC=`uname -m`

curl -f http://get.zedapp.org/zedrem-$OS-$PROC > zedrem 2> /dev/null
if [ $? == 0 ]; then
    chmod +x zedrem

    echo "Done, zedrem downloaded into current directory, to start: ./zedrem"
    echo "For help: ./zedrem --help"
else
    echo "It appears there is no pre-compiled version of zedrem available."
    echo "Please compile your own, see: https://github.com/zedapp/zedrem for instructions"
fi
