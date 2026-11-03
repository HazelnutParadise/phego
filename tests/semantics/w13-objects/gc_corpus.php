<?php
// REQ-020/021; T-039-042. Deterministic graphs exercise root reuse and
// mixed live/cyclic owners; operation arithmetic fits both integer widths.
for($case=1;$case<=64;$case++) {
    $tests['gc-sequence-'.$case]=function() use($case) {
        $GLOBALS['gc_events']=[];$vars=[];$seed=$case;$serial=0;$counts=[];
        for($step=0;$step<40;$step++) {
            $seed=($seed*73+19)%65521;
            $op=$seed%5;$a=intdiv($seed,5)%8;$b=intdiv($seed,40)%8;
            if($op===0) {$temp=new W13GC('s'.$case.'-'.(++$serial));$vars[$a]=$temp;unset($temp);}
            elseif($op===1&&isset($vars[$a])) {$from=$vars[$a];$to=$vars[$b]??null;$from->{'p'.($seed%3)}=$to;unset($from,$to);}
            elseif($op===2) {unset($vars[$a]);}
            elseif($op===3) {$counts[]=gc_collect_cycles();}
            elseif($op===4&&isset($vars[$b])) {$vars[$a]=$vars[$b];}
        }
        $counts[]=gc_collect_cycles();unset($vars);$counts[]=gc_collect_cycles();
        return [$counts,$GLOBALS['gc_events']];
    };
}
