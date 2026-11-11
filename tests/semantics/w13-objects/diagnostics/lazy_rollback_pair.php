<?php
// Diagnostic only: consecutive rollback cases corrupt the pinned PHP heap.
function attempt($fn){try{return $fn();}catch(Throwable $e){return [$e::class,$e->getMessage()];}}
$tests=[];
require dirname(__DIR__).'/review.php';
foreach(['lazy-rollback-write-declared','lazy-rollback-write-dynamic'] as $name){
    echo 'before=',$name,"\n";
    gc_collect_cycles();
    echo json_encode(attempt($tests[$name]),JSON_THROW_ON_ERROR),"\n";
}
echo "before-final-gc\n";
echo gc_collect_cycles(),"\n";
