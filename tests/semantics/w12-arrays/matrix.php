<?php
// Original fixtures: REQ-007/016/017; T-013/014/031/032/033/034.
// Each operation is interpreted by both PHP and Go. Results contain ordered,
// typed values, diagnostics and caught Throwables; no engine is used by Go.
function enc($v) {
    if ($v === null) return ['n'];
    if (is_bool($v)) return ['b', $v];
    if (is_int($v)) return ['i', (string)$v];
    if (is_float($v)) return ['f', bin2hex(pack('E', $v))];
    if (is_string($v)) return ['s', bin2hex($v)];
    $out = [];
    foreach ($v as $k => $x) $out[] = [enc($k), enc($x)];
    return ['a', $out];
}
function val($x, &$v) {
    if (is_array($x)) {
        return match ($x[0]) {
            'var' => $v[$x[1]], 'float' => match ($x[1]) {
                'inf' => INF, '-inf' => -INF, 'nan' => NAN,
                default => (float)$x[1],
            }, 'max' => PHP_INT_MAX, 'min' => PHP_INT_MIN,
            'array' => [], 'object' => new stdClass,
        };
    }
    return $x;
}
$cases = [];
$keys = [null, true, false, 0, -7, '', '0', '-0', '00', '01', '+1', ' 1', '1 ', '1.0', '1e2', "1\0", "\xff", '2147483647', '2147483648', '-2147483648', '-2147483649', '9223372036854775807', '9223372036854775808'];
foreach (['0', '-0', '1.5', '-1.5', '2147483648', '4294967295', '9223372036854775808', '1e20', 'inf', '-inf', 'nan'] as $f) $keys[] = ['float', $f];
foreach ($keys as $i => $k) {
    $cases['key-'.$i] = [['set','a',$k,7], ['get','a',$k], ['emit','a'], ['delete','a',$k], ['emit','a']];
}
$cases['order'] = [['append','a',10],['append','a',20],['set','a','x',30],['delete','a',0],['set','a',0,40],['append','a',50],['emit','a']];
$cases['negative-next'] = [['set','a',-7,1],['append','a',2],['delete','a',-6],['append','a',3],['emit','a']];
$cases['max-next'] = [['set','a',['max'],1],['append','a',2],['delete','a',['max']],['append','a',3],['append','a',4],['emit','a']];
$cases['min-next'] = [['set','a',['min'],1],['append','a',2],['emit','a']];
$cases['cow-alias'] = [['append','a',1],['bind_element','r','a',0],['copy','b','a'],['assign','r',2],['set','b',1,3],['emit','a'],['emit','b'],['unset','r'],['copy','c','a'],['set','c',0,4],['emit','a'],['emit','c']];
$cases['rebind'] = [['assign','x',1],['bind','y','x'],['bind','z','y'],['assign','y',2],['unset','x'],['assign','x',3],['emit','y'],['emit','z'],['emit','x']];
$cases['nested-copy'] = [['append','a',"a\0b"],['append','a',['var','a']],['copy','b','a'],['set','b',0,5],['emit','a'],['emit','b']];
$cases['foreach-value'] = [['append','a',1],['append','a',2],['append','a',3],['foreach_value','a','v',[
    ['emit','v'],['at',0,[['set','a',1,20],['delete','a',2],['append','a',4]]]
]],['emit','a'],['emit','v']];
$cases['foreach-ref'] = [['append','a',1],['append','a',2],['append','a',3],['foreach_ref','a','v',[
    ['emit','v'],['at',0,[['delete','a',1],['append','a',4]]],['assign','v',9]
]],['assign','v',10],['emit','a']];
$cases['foreach-ref-copy'] = [['append','a',1],['append','a',2],['foreach_ref','a','v',[
    ['emit','v'],['at',0,[['copy','b','a'],['append','a',3],['copy','c','a'],['set','a','x',4]]]
]],['emit','a'],['emit','b'],['emit','c']];
$cases['foreach-value-alias'] = [['append','a',1],['append','a',2],['bind_element','r','a',1],['foreach_value','a','v',[
    ['emit','v'],['at',0,[['assign','r',8]]]
]],['emit','a']];
$cases['foreach-nonarray'] = [['assign','a',null],['foreach_value','a','v',[]],['assign','a',7],['foreach_ref','a','v',[]]];
$cases['pointers'] = [['pointer','a','current'],['append','a',10],['append','a',20],['pointer','a','key'],['pointer','a','next'],['copy','b','a'],['pointer','b','next'],['pointer','b','key'],['pointer','a','current'],['pointer','b','prev'],['pointer','b','reset'],['delete','b',0],['pointer','b','current'],['pointer','b','end'],['pointer','b','next'],['copy','c','b'],['set','c','x',30],['pointer','c','current'],['pointer','c','key'],['emit','c']];
$cases['pointer-foreach'] = [['append','a',1],['append','a',2],['pointer','a','end'],['foreach_ref','a','v',[['pointer','a','key']]],['pointer','a','key']];
$cases['invalid-key'] = [['set','a',['array'],1],['append','a',2],['get','a',['array']],['delete','a',['array']],['emit','a']];
$cases['foreach-other-copy'] = [['append','a',1],['append','a',2],['foreach_ref','a','v',[
    ['emit','v'],['at',0,[['copy','b','a'],['append','b',3]]]
]],['emit','a'],['emit','b']];
$cases['foreach-replace-pointer'] = [['append','a',1],['append','a',2],['append','b',7],['append','b',8],['pointer','b','end'],['foreach_ref','a','v',[
    ['emit','v'],['at',0,[['copy','a','b']]]
]],['emit','a']];
$cases['foreach-nested'] = [['append','a',1],['append','a',2],['foreach_ref','a','v',[
    ['emit','v'],['foreach_ref','a','w',[['emit','w']]]
]],['assign','w',3],['emit','a']];
$cases['foreach-lingering'] = [['append','a',1],['append','a',2],['append','a',3],['foreach_ref','a','v',[]],['foreach_value','a','v',[['emit','v']]],['emit','a']];
$cases['indexed-tail-pointer'] = [['set','a','x',1],['append','a',2],['pointer','a','end'],['delete','a',0],['pointer','a','prev'],['append','a',3],['pointer','a','current'],['pointer','a','reset'],['pointer','a','next'],['pointer','a','prev']];
$cases['empty-pointer'] = [['pointer','a','prev'],['append','a',1],['pointer','a','current'],['pointer','a','prev'],['pointer','a','next'],['pointer','a','current'],['append','a',2],['pointer','a','current'],['pointer','a','reset']];
$cases['object-key'] = [['append','a',1],['set','a',['object'],2],['get','a',['object']],['delete','a',['object']],['emit','a']];
$cases['foreach-rebind-variable'] = [['append','a',1],['append','a',2],['append','b',8],['foreach_ref','a','v',[
    ['emit','v'],['at',0,[['bind','a','b']]]
]],['emit','a']];
$cases['foreach-change-type'] = [['append','a',1],['append','a',2],['foreach_ref','a','v',[['assign','a',7]]],['emit','a']];
$cases['pointer-scalar'] = [['assign','a',7],['pointer','a','current'],['pointer','a','next']];
$cases['invalid-pointer-tail-delete'] = [['append','a',1],['append','a',2],['append','a',3],['pointer','a','end'],['pointer','a','next'],['delete','a',2],['append','a',4],['pointer','a','key'],['pointer','a','current']];
$cases['pointer-tail-holes'] = [['set','a','a',1],['set','a','b',2],['set','a','c',3],['delete','a','b'],['pointer','a','end'],['delete','a','c'],['append','a',4],['pointer','a','key'],['pointer','a','current']];
$cases['foreach-sort'] = [['append','a',3],['append','a',1],['append','a',2],['foreach_ref','a','v',[
    ['emit','v'],['at',0,[['sort','a','usort',false]]]
]],['emit','a']];
foreach (['usort','uasort'] as $sort) {
    $cases[$sort] = [['set','a','x',3],['set','a',5,1],['set','a','y',1],['pointer','a','end'],['copy','b','a'],['sort','a',$sort,false],['emit','a'],['emit','b'],['pointer','a','key'],['append','a',9],['emit','a']];
    $cases[$sort.'-reentry'] = [['append','a',3],['append','a',1],['append','a',2],['sort','a',$sort,true],['emit','a']];
    $cases[$sort.'-throw'] = [['append','a',3],['append','a',1],['append','a',2],['sort','a',$sort,'throw'],['emit','a']];
    $cases[$sort.'-throw-keys'] = [['set','a','x',3],['set','a',5,1],['pointer','a','end'],['sort','a',$sort,'throw'],['emit','a'],['pointer','a','key'],['append','a',9],['emit','a']];
    $cases[$sort.'-empty'] = [['assign','a',['array']],['sort','a',$sort,false],['emit','a'],['append','a',7],['emit','a']];
}
function execute($ops, &$v, &$out, $iteration = -1) {
    foreach ($ops as $op) {
        $diags = [];
        set_error_handler(function($n,$m) use (&$diags) { $diags[] = [$n,$m]; return true; });
        try {
            switch ($op[0]) {
                case 'set': $v[$op[1]][val($op[2],$v)] = val($op[3],$v); break;
                case 'append': $v[$op[1]][] = val($op[2],$v); break;
                case 'assign': $v[$op[1]] = val($op[2],$v); break;
                case 'copy': $v[$op[1]] = $v[$op[2]]; break;
                case 'bind': $v[$op[1]] =& $v[$op[2]]; break;
                case 'bind_element': $v[$op[1]] =& $v[$op[2]][val($op[3],$v)]; break;
                case 'unset': unset($v[$op[1]]); break;
                case 'delete': unset($v[$op[1]][val($op[2],$v)]); break;
                case 'get': $out[] = ['value', enc($v[$op[1]][val($op[2],$v)])]; break;
                case 'emit': $out[] = ['value', enc($v[$op[1]])]; break;
                case 'at': if ($iteration === $op[1]) execute($op[2],$v,$out,$iteration); break;
                case 'sort':
                    $once = false;
                    ($op[2])($v[$op[1]],function($a,$b) use (&$v,$op,&$once) {
                        if ($op[3] === 'throw') { $v[$op[1]][] = 99; throw new Exception('comparison failed'); }
                        if ($op[3] && !$once) { $once=true; $v[$op[1]][] = 99; }
                        return $a <=> $b;
                    }); break;
                case 'pointer':
                    $v[$op[1]] ??= [];
                    $out[] = ['value', enc(($op[2])($v[$op[1]]))]; break;
                case 'foreach_value':
                    $i = 0;
                    foreach ($v[$op[1]] as $v[$op[2]]) {
                        if ($i > 32) throw new Exception('fixture iteration bound');
                        execute($op[3],$v,$out,$i++);
                    } break;
                case 'foreach_ref':
                    $i = 0;
                    foreach ($v[$op[1]] as &$v[$op[2]]) {
                        if ($i > 32) throw new Exception('fixture iteration bound');
                        execute($op[3],$v,$out,$i++);
                    } break;
            }
        } catch (Throwable $e) { $out[] = ['error',get_class($e),$e->getMessage()]; }
        finally { restore_error_handler(); }
        foreach ($diags as $d) $out[] = ['diagnostic',...$d];
    }
}
$out = ['php'=>PHP_VERSION,'int_size'=>PHP_INT_SIZE,'cases'=>[]];
foreach ($cases as $name=>$ops) {
    $v = []; $events = [];
    execute($ops,$v,$events);
    $out['cases'][] = ['name'=>$name,'events'=>$events];
}
// Operations travel separately as JSON with strings encoded by enc().
function input_enc($x) { return is_array($x) ? array_map('input_enc',$x) : enc($x); }
foreach ($out['cases'] as $i=>&$c) $c['operations'] = input_enc(array_values($cases)[$i]);
unset($c);
echo json_encode($out, JSON_THROW_ON_ERROR|JSON_UNESCAPED_SLASHES), "\n";
