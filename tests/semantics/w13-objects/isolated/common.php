<?php
// REQ-018/020; T-035/036/039/040. Preserve the full single-case observation.
function attempt($fn){try{return $fn();}catch(Throwable $e){return [$e::class,$e->getMessage()];}}
$tests=[];
require dirname(__DIR__).'/review.php';
$diagnostics=[];
set_error_handler(function($level,$message)use(&$diagnostics){$diagnostics[]=[$level,$message];return true;});
$result=attempt($tests[$case]);
restore_error_handler();
echo json_encode(['php'=>PHP_VERSION,'int_size'=>PHP_INT_SIZE,'cases'=>[$case=>['result'=>$result,'diagnostics'=>$diagnostics]]],JSON_THROW_ON_ERROR),"\n";
