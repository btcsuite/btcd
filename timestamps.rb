#!/usr/bin/ruby

require 'date'

HOUR = 60*60

previous = -1
previousTime = nil

blocks = []
blockMap = {}

ARGF.each_line do |line|
  blockHash, prevBlock, timestamp = line.match(/^XXXXX BlockHash: ([^ ]*) PrevBlock: ([^ ]*) Timestamp: (.*)/).captures

  datetime = DateTime.parse(timestamp)
  current = datetime.strftime("%s").to_i

  if previous == -1 
    previous = current
    previousTime = datetime
  else
    blockMap[blockHash] = datetime
  end

  blocks << [blockHash, prevBlock]

  if (current - previous) >= (2 * HOUR) 
    #puts "FOUND 2 HOUR DIFFERENCE of #{current - previous} previousTime #{previousTime} currentTime #{datetime}"
  end

  previous = datetime.strftime("%s").to_i
  previousTime = datetime
end

# this is to double check that the prev block and the current block are in succession (that the input log isn't faulty)
# turns out the input isn't faulty however using this just in case...
blocks.each do |block|
  blockHash, prevBlock = block

  if blockMap.has_key?(blockHash) && blockMap.has_key?(prevBlock)
    current = blockMap[blockHash].strftime("%s").to_i
    previous = blockMap[prevBlock].strftime("%s").to_i

    if (current - previous) >= (2 * HOUR) 
      puts "FOUND 2 HOUR DIFFERENCE of #{current - previous} previousTime #{blockMap[prevBlock]} currentTime #{blockMap[blockHash]}"
    end
  end
end

