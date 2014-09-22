#!/usr/bin/env bash

OS=`uname -s`
PROC=`uname -m`

URL="http://get.zedapp.org/zedrem-$OS-$PROC"

curl -f $URL > zedrem 2> /dev/null
if [ $? == 0 ]; then
    chmod +x zedrem

    echo "Done, zedrem downloaded into current directory, to start: ./zedrem"
    echo "For help: ./zedrem --help"
else
    echo "It appears there is no pre-compiled version of zedrem available for your platform: $OS-$PROC."
    echo "I expected it to be located here: $URL"
    echo "Please compile your own, see: https://github.com/zedapp/zedrem for instructions"
fi
