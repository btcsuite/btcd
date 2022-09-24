#! /bin/bash

read -r -d '' help << EOM
$0 - helper script for displaying miner of a mined block.

Options:

    -h Display this message.

    --height Specify blockheight.
    --hash Specify blockhash.
EOM

while getopts ":h-:" optchar; do
	case "${optchar}" in
		-)
			case "${OPTARG}" in
				hash)
					blockhash="${!OPTIND}"; OPTIND=$(( $OPTIND + 1 ))
					;;
				height)
					blockheight="${!OPTIND}"; OPTIND=$(( $OPTIND + 1 ))
					blockhash=$(lbcctl getblockhash ${blockheight})
					;;
				*) echo "Unknown long option --${OPTARG}" >&2; exit -2 ;;
                        esac
        ;;
		h) printf "${help}\n\n"; exit 2;;
		*) echo "Unknown option -${OPTARG}" >&2; exit -2;;
	esac
done


block=$(lbcctl getblock $blockhash)
blockheight=$(lbcctl getblock $blockhash | jq -r .height)

coinbase_txid=$(echo ${block} | jq -r '.tx[0]')
coinbase_raw=$(lbcctl getrawtransaction ${coinbase_txid} 1)
coinbase=$(echo ${coinbase_raw} | jq '.vin[0].coinbase')
miner=$(echo ${coinbase} | grep -o '2f.*2f' | xxd -r -p | strings)

echo ${blockheight}: ${blockhash}: ${miner}
