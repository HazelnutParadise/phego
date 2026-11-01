<?php
// REQ-017; T-033/T-034: exercise native PHP sort behavior as the oracle.
function w12_sort_ints($n, $shape) {
    $values = [];
    if ($shape === 'descending') {
        for ($i = $n; $i > 0; $i--) $values[] = $i;
        return $values;
    }
    if ($shape === 'duplicates') {
        for ($i = 0; $i < $n; $i++) $values[] = (($i * 7) % 11) - 5;
        return $values;
    }
    for ($i = 0; $i < $n; $i++) $values[] = $i;
    $state = 17;
    for ($i = $n - 1; $i > 0; $i--) {
        $state = ($state * 25173 + 13849) % 65521;
        $j = $state % ($i + 1);
        [$values[$i], $values[$j]] = [$values[$j], $values[$i]];
    }
    return $values;
}

function w12_sort_case($function, $mode, $values, $throwAt = 0, $keys = null, $nested = false) {
    $ops = [];
    if ($nested) $ops[] = ['append', 'inner', 1];
    foreach ($values as $i => $value) {
        if ($keys === null) $ops[] = ['append', 'a', $value];
        else $ops[] = ['set', 'a', $keys[$i], $value];
    }
    $ops[] = ['sort_trace', 'a', $function, $mode, $throwAt];
    $ops[] = ['emit', 'a'];
    $ops[] = ['pointer', 'a', 'key'];
    $ops[] = ['append', 'a', 999999];
    $ops[] = ['emit', 'a'];
    return $ops;
}

function w12_sort_cases() {
    $cases = [];
    $shapes = ['descending', 'permuted', 'duplicates'];

    foreach (['usort', 'uasort'] as $function) {
        foreach (['one', 'minus', 'zero'] as $modeIndex => $mode) {
            for ($n = 2; $n <= 40; $n++) {
                $shape = $shapes[($n + $modeIndex) % count($shapes)];
                $cases["$function-$mode-$shape-$n"] =
                    w12_sort_case($function, $mode, w12_sort_ints($n, $shape));
            }
        }
    }

    $traceLengths = array_merge(range(2, 40), [65, 127, 1024, 1025, 2048]);
    foreach (['usort', 'uasort'] as $function) {
        foreach ($traceLengths as $n) {
            $shape = $shapes[$n % count($shapes)];
            $cases["$function-normal-$shape-$n"] =
                w12_sort_case($function, 'normal', w12_sort_ints($n, $shape));
        }
    }

    foreach ([17, 40] as $n) {
        $cases["usort-cycle-$n"] =
            w12_sort_case('usort', 'cycle', w12_sort_ints($n, 'permuted'));
    }

    $throwLengths = [3, 6, 16, 17, 40, 65, 1025];
    foreach (['usort', 'uasort'] as $function) {
        foreach ($throwLengths as $n) {
            foreach ([
                'first' => 1,
                'second' => 2,
                'half' => intdiv($n, 2),
            ] as $point => $throwAt) {
                $shape = match ($point) {
                    'first' => 'descending',
                    'second' => 'duplicates',
                    default => 'permuted',
                };
                $cases["$function-throw-$point-$n"] =
                    w12_sort_case($function, 'normal', w12_sort_ints($n, $shape), $throwAt);
            }
        }
    }

    $sparseKeys = [7, 'word', 2, 'x', 19, '03'];
    $sparseValues = [5, 1, 4, 0, 3, 2];
    $cases['uasort-sparse-string-keys'] =
        w12_sort_case('uasort', 'normal', $sparseValues, 0, $sparseKeys);
    $cases['uasort-throw-sparse-string-keys'] =
        w12_sort_case('uasort', 'normal', $sparseValues, 2, $sparseKeys);

    $sample = ['abc', 0, '1', true, '10', null, 'z', ['float', '2.5'], '', ['var', 'inner']];
    foreach (['sort', 'asort'] as $function) {
        $cases["mixed-sample-$function"] =
            w12_sort_case($function, 'normal', $sample, 0, null, true);
    }

    $mixedValues = [
        null, false, true, -3, 0, 1, 2, 10,
        ['float', '-1.5'], ['float', '2.5'], ['array'], ['var','inner'],
        '', '0', '1', '10', 'abc', 'z',
    ];
    for ($case = 0; $case < 32; $case++) {
        $length = 8 + (($case * 37 + 19) % 57);
        $state = ($case * 811 + 17) % 65521;
        $values = [];
        for ($i = 0; $i < $length; $i++) {
            $state = ($state * 25173 + 13849) % 65521;
            $values[] = $mixedValues[$state % count($mixedValues)];
        }
        foreach (['sort', 'asort'] as $function) {
            $cases[sprintf('mixed-random-%02d-%s', $case, $function)] =
                w12_sort_case($function, 'normal', $values, 0, null, true);
        }
    }

    foreach (['usort','uasort'] as $function) {
        foreach (['bool','boolfalse','booltrue','boolnested'] as $mode) {
            foreach ([2,3,6,17,40] as $n) {
                $cases["$function-$mode-$n"]=w12_sort_case($function,$mode,w12_sort_ints($n,'permuted'));
            }
        }
        $cases["$function-bool-reverse-throw"]=w12_sort_case($function,'bool',[1,3,2],2);
    }
    // REQ-017; T-033/T-034: native sort mutates a separated input; user sort
    // commits a private copy. Empty COW and tiny arrays expose that distinction.
    foreach (['usort','uasort','sort','asort'] as $function) {
        foreach (['single','single-exhausted','empty-new','empty-deleted','empty-shared'] as $shape) {
            $ops=[['assign','a',['array']]];
            if ($shape!=='empty-new') $ops[]=['set','a',3,1];
            if ($shape==='single-exhausted') $ops[]=['pointer','a','next'];
            if ($shape==='empty-deleted'||$shape==='empty-shared') $ops[]=['delete','a',3];
            if ($shape==='empty-shared') $ops[]=['copy','b','a'];
            $ops[]=['sort_trace','a',$function,'normal',0];
            $ops[]=['pointer','a','key'];
            $ops[]=['set','a',6,9];
            $ops[]=['pointer','a','end'];$ops[]=['pointer','a','next'];
            $ops[]=['set','a','x',7];$ops[]=['pointer','a','key'];$ops[]=['emit','a'];
            $cases["$function-$shape-position"]=$ops;
        }
        $cases["$function-foreach-mixed-holes"]=[['set','a','x',3],['set','a','y',1],['set','a','z',2],['set','a','w',4],['delete','a','y'],['foreach_ref','a','v',[
            ['emit','v'],['at',1,[['delete','a','x'],['sort_trace','a',$function,'normal',0]]]
        ]],['emit','a'],['emit','v']];
    }
    foreach (['sort','asort'] as $function) $cases["$function-recursive-array-index"]=[['sort_recursive','a',$function]];
    foreach (['usort','uasort'] as $function) {
        foreach (['boolref','boolwarntrue','boolwarnfalse'] as $mode) {
            foreach([2,3,6,17] as $n) $cases["$function-$mode-$n"]=w12_sort_case($function,$mode,w12_sort_ints($n,'permuted'));
        }
        $cases["$function-boolref-return-throw"]=w12_sort_case($function,'boolref',[1,3,2],2);
    }
    foreach (['usort','uasort'] as $function) {
        foreach (['object','objectwarn','float','floatwarn'] as $mode) {
            foreach([3,17] as $n) $cases["$function-$mode-$n"]=w12_sort_case($function,$mode,w12_sort_ints($n,'permuted'));
        }
    }
    // REQ-017; T-033/T-034: bool retry reloads externally referenced values.
    foreach (['usort','uasort'] as $function) foreach (['boolmutate','booldiagmutate'] as $mode) {
        $cases["$function-$mode-live-retry"]=[['append','a',1],['append','a',2],['bind_element','r','a',0],['sort_trace','a',$function,$mode,0],['emit','a'],['emit','r']];
    }
    return $cases;
}

// Inspect bounded type/key observations, never recursively encode the cycle.
function w12_sort_recursive(&$v,$op,&$out) {
    $v[$op[1]]=[];$v[$op[1]][0]=[1,2];$v[$op[1]][1]=&$v[$op[1]];
    ($op[2])($v[$op[1]]);
    foreach($v[$op[1]] as $k=>$item) $out[]=['recursive-key',enc($k),gettype($item[0])];
}

function w12_sort_trace(&$v, $op, &$out) {
    $function = $op[2];
    if ($function === 'sort') {
        sort($v[$op[1]]);
        return;
    }
    if ($function === 'asort') {
        asort($v[$op[1]]);
        return;
    }

    $calls = 0;
    $mode = $op[3];
    $throwAt = $op[4];
    $compare = function($a, $b) use (&$v, &$out, &$calls, $mode, $throwAt) {
        $out[] = ['compare', enc($a), enc($b)];
        $calls++;
        if ($throwAt !== 0 && $calls === $throwAt) {
            throw new Exception('comparison failed');
        }
        switch ($mode) {
            case 'one': return 1;
            case 'minus': return -1;
            case 'zero': return 0;
            case 'normal': return $a <=> $b;
            case 'bool': return $a > $b;
            case 'boolfalse': return false;
            case 'booltrue': return true;
            case 'boolwarntrue': return true;
            case 'boolwarnfalse': return false;
            case 'object': case 'objectwarn': return new stdClass;
            case 'float': case 'floatwarn': return INF;
            case 'boolmutate': case 'booldiagmutate':
                if ($calls===1) { if($mode==='boolmutate')$v['r']=10;return false; }
                return $a-$b;
            case 'boolnested':
                if ($calls===2) { $inner=[2,1]; usort($inner,fn($x,$y)=>$x<=>$y); }
                return $a > $b;
            case 'cycle':
                if ($a === $b) return 0;
                if (($a % 3 + 1) % 3 === $b % 3) return -1;
                return 1;
        }
        throw new Exception('unsupported comparison mode');
    };

    if($mode==='boolref') {
        $returnSlot=false;
        $compare=function &($a,$b)use(&$out,&$calls,&$returnSlot,$throwAt){
            $out[]=['compare',enc($a),enc($b)];$calls++;
            if($throwAt!==0&&$calls===$throwAt)throw new Exception('comparison failed');
            $returnSlot=$a>$b;return$returnSlot;
        };
    }
    $warningThrows=in_array($mode,['boolwarntrue','boolwarnfalse','objectwarn','floatwarn'],true);
    if($warningThrows)set_error_handler(function($level,$message)use(&$out){$out[]=['diagnostic',$level,$message];throw new Exception('comparison warning failed');});
    if($mode==='booldiagmutate')set_error_handler(function($level,$message)use(&$v,&$out){$out[]=['diagnostic',$level,$message];$v['r']=10;});
    try {
        if ($function === 'usort') usort($v[$op[1]], $compare);
        else uasort($v[$op[1]], $compare);
    } finally {if($warningThrows||$mode==='booldiagmutate')restore_error_handler();}
}
