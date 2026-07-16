git clone https://github.com/disgoorg/godave.git

cd godave
readonly VERSION=$(cat libdave/release.txt)

cd scripts
./libdave_install.sh ${VERSION}