<?php
// Diagnostic only: execute one rollback case without the normal matrix loop.
function attempt($fn){try{return $fn();}catch(Throwable $e){return [$e::class,$e->getMessage()];}}
$tests=[];
require dirname(__DIR__).'/review.php';
echo 'case=',$case,' bits=',PHP_INT_SIZE*8,"\n";
$result=attempt($tests[$case]);
echo json_encode($result,JSON_THROW_ON_ERROR),"\n";
echo "before-gc\n";
$count=gc_collect_cycles();
echo 'after-gc=',$count,"\n";
