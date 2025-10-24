<?php

use think\Db;

define('MAILFLOW_DEBUG', true);

function mailflow_debug($message, $data = null) {
    if (!MAILFLOW_DEBUG) return;
    $log = '[MailFlow-DEBUG] ' . $message;
    if ($data !== null) {
        $log .= ' | Data: ' . json_encode($data, JSON_UNESCAPED_UNICODE);
    }
    error_log($log);
}

// 插件元数据信息
function mailflow_MetaData()
{
    return [
        'DisplayName' => 'MailFlow邮件服务插件 by xkatld',
        'APIVersion'  => '1.0.0',
        'HelpDoc'     => 'https://github.com/xkatld/MailFlow',
    ];
}

// 产品配置选项定义
function mailflow_ConfigOptions()
{
    return [
        'config_mode' => [
            'type'        => 'dropdown',
            'name'        => '配置模式',
            'description' => '选择使用预设套餐或自定义配置',
            'default'     => 'plan',
            'key'         => 'config_mode',
            'options'     => [
                'plan' => '预设套餐',
                'custom' => '自定义配置',
            ],
        ],
        'plan_code' => [
            'type'        => 'text',
            'name'        => '套餐代码',
            'description' => '套餐代码（配置模式为预设套餐时使用）',
            'default'     => 'basic',
            'key'         => 'plan_code',
        ],
        'minute_limit' => [
            'type'        => 'text',
            'name'        => '每分钟限制',
            'description' => '每分钟发送限制（自定义模式，0=无限）',
            'default'     => '100',
            'key'         => 'minute_limit',
        ],
        'daily_limit' => [
            'type'        => 'text',
            'name'        => '每日限制',
            'description' => '每日发送限制（自定义模式，0=无限）',
            'default'     => '5000',
            'key'         => 'daily_limit',
        ],
        'weekly_limit' => [
            'type'        => 'text',
            'name'        => '每周限制',
            'description' => '每周发送限制（自定义模式，0=无限）',
            'default'     => '30000',
            'key'         => 'weekly_limit',
        ],
        'monthly_limit' => [
            'type'        => 'text',
            'name'        => '每月限制',
            'description' => '每月发送限制（自定义模式，0=无限）',
            'default'     => '100000',
            'key'         => 'monthly_limit',
        ],
        'total_limit' => [
            'type'        => 'text',
            'name'        => '总量限制',
            'description' => '总发送限制（0=无限）',
            'default'     => '0',
            'key'         => 'total_limit',
        ],
    ];
}

// 测试API连接
function mailflow_TestLink($params)
{
    mailflow_debug('开始测试API连接', $params);

    $res = mailflow_Curl($params, '/api/public/plans', [], 'GET');

    if ($res === null) {
        return [
            'status' => 200,
            'data'   => [
                'server_status' => 0,
                'msg'           => "连接失败: 无法连接到MailFlow服务器"
            ]
        ];
    } elseif (isset($res['error'])) {
        return [
            'status' => 200,
            'data'   => [
                'server_status' => 0,
                'msg'           => "连接失败: " . $res['error']
            ]
        ];
    } elseif (is_array($res) && !isset($res['error'])) {
        return [
            'status' => 200,
            'data'   => [
                'server_status' => 1,
                'msg'           => "连接成功，可用套餐: " . count($res)
            ]
        ];
    } else {
        return [
            'status' => 200,
            'data'   => [
                'server_status' => 0,
                'msg'           => "连接失败: 响应格式异常"
            ]
        ];
    }
}

// 客户区页面定义
function mailflow_ClientArea($params)
{
    return [
        'info'  => ['name' => 'API Key信息'],
        'docs'  => ['name' => '使用文档'],
        'quota' => ['name' => '配额详情'],
        'stats' => ['name' => '使用统计'],
        'logs'  => ['name' => '发送日志'],
    ];
}

// 客户区输出处理
function mailflow_ClientAreaOutput($params, $key)
{
    mailflow_debug('ClientAreaOutput调用', ['key' => $key, 'action' => $_GET['action'] ?? null]);

    if (isset($_GET['action'])) {
        $action = $_GET['action'];
        mailflow_debug('处理API请求', ['action' => $action, 'username' => $params['username'] ?? null]);

        if (empty($params['username'])) {
            header('Content-Type: application/json');
            echo json_encode(['code' => 400, 'msg' => 'API Key未设置']);
            exit;
        }

        $apiKey = $params['username'];
        $apiKeyID = mailflow_GetAPIKeyID($params);

        if ($action === 'getquota') {
            if (!$apiKeyID) {
                header('Content-Type: application/json');
                echo json_encode(['code' => 404, 'msg' => 'API Key不存在']);
                exit;
            }

            $res = mailflow_Curl($params, "/admin/api/keys/{$apiKeyID}/quota", [], 'GET', true);
            header('Content-Type: application/json');
            echo json_encode($res ?? ['code' => 500, 'msg' => '获取配额失败']);
            exit;
        }

        if ($action === 'getstats') {
            $res = mailflow_Curl($params, '/api/v1/usage', [], 'GET', false, $apiKey);
            header('Content-Type: application/json');
            echo json_encode($res ?? ['code' => 500, 'msg' => '获取统计失败']);
            exit;
        }

        if ($action === 'getlogs') {
            $page = intval($_GET['page'] ?? 1);
            $pageSize = intval($_GET['page_size'] ?? 20);
            $status = $_GET['status'] ?? '';

            $queryParams = "page={$page}&page_size={$pageSize}";
            if ($status) {
                $queryParams .= "&status={$status}";
            }

            $res = mailflow_Curl($params, "/api/v1/logs?{$queryParams}", [], 'GET', false, $apiKey);
            header('Content-Type: application/json');
            echo json_encode($res ?? ['code' => 500, 'msg' => '获取日志失败']);
            exit;
        }
    }

    if ($key == 'info') {
        return [
            'template' => 'templates/info.html',
            'vars'     => [
                'api_key' => $params['username'] ?? '',
                'api_url' => 'http://' . $params['server_ip'] . ':' . $params['port'],
                'key_name' => $params['domain'] ?? '',
            ],
        ];
    }

    if ($key == 'docs') {
        return [
            'template' => 'templates/docs.html',
            'vars'     => [
                'api_key' => $params['username'] ?? '',
                'api_url' => 'http://' . $params['server_ip'] . ':' . $params['port'],
                'key_name' => $params['domain'] ?? '',
            ],
        ];
    }

    if ($key == 'quota') {
        return [
            'template' => 'templates/quota.html',
            'vars'     => [],
        ];
    }

    if ($key == 'stats') {
        return [
            'template' => 'templates/stats.html',
            'vars'     => [],
        ];
    }

    if ($key == 'logs') {
        return [
            'template' => 'templates/logs.html',
            'vars'     => [],
        ];
    }
}

// 允许客户端调用的函数列表
function mailflow_AllowFunction()
{
    return [
        'client' => [],
    ];
}

// 创建API Key
function mailflow_CreateAccount($params)
{
    mailflow_debug('开始创建API Key', ['domain' => $params['domain']]);

    $configMode = $params['configoptions']['config_mode'] ?? 'plan';
    
    $data = [
        'name' => $params['domain'] ?? 'API Key',
        'total_limit' => intval($params['configoptions']['total_limit'] ?? 0),
    ];

    if ($configMode === 'plan') {
        $planCode = $params['configoptions']['plan_code'] ?? 'basic';
        
        $plans = mailflow_Curl($params, '/api/public/plans', [], 'GET');
        if (!is_array($plans)) {
            return ['status' => 'error', 'msg' => '无法获取套餐列表'];
        }

        $planID = null;
        foreach ($plans as $plan) {
            if ($plan['code'] === $planCode) {
                $planID = $plan['id'];
                break;
            }
        }

        if (!$planID) {
            return ['status' => 'error', 'msg' => "套餐 {$planCode} 不存在"];
        }

        $data['plan_id'] = $planID;
    } else {
        $data['minute_limit'] = intval($params['configoptions']['minute_limit'] ?? 100);
        $data['daily_limit'] = intval($params['configoptions']['daily_limit'] ?? 5000);
        $data['weekly_limit'] = intval($params['configoptions']['weekly_limit'] ?? 30000);
        $data['monthly_limit'] = intval($params['configoptions']['monthly_limit'] ?? 100000);
    }

    mailflow_debug('发送创建请求', $data);
    $res = mailflow_Curl($params, '/admin/api/keys', $data, 'POST', true);
    mailflow_debug('创建响应', $res);

    if (isset($res['key'])) {
        $update = [
            'username'     => $res['key'],
            'domain'       => $res['name'],
            'dedicatedip'  => $params['server_ip'],
            'domainstatus' => 'Active',
        ];

        try {
            Db::name('host')->where('id', $params['hostid'])->update($update);
            mailflow_debug('数据库更新成功', $update);
        } catch (\Exception $e) {
            return ['status' => 'error', 'msg' => '创建成功，但同步数据到面板失败: ' . $e->getMessage()];
        }

        return ['status' => 'success', 'msg' => 'API Key创建成功'];
    } else {
        return ['status' => 'error', 'msg' => $res['error'] ?? 'API Key创建失败'];
    }
}

// 同步API Key信息
function mailflow_Sync($params)
{
    mailflow_debug('开始同步API Key信息', ['username' => $params['username']]);

    if (empty($params['username'])) {
        return ['status' => 'error', 'msg' => 'API Key未设置'];
    }

    $apiKeyID = mailflow_GetAPIKeyID($params);
    if (!$apiKeyID) {
        return ['status' => 'error', 'msg' => 'API Key不存在'];
    }

    $res = mailflow_Curl($params, '/admin/api/keys', [], 'GET', true);

    if (is_array($res)) {
        foreach ($res as $keyInfo) {
            if ($keyInfo['id'] == $apiKeyID) {
                try {
                    $update = [
                        'domain' => $keyInfo['name'],
                    ];
                    Db::name('host')->where('id', $params['hostid'])->update($update);
                } catch (\Exception $e) {
                    mailflow_debug('同步数据库失败', ['error' => $e->getMessage()]);
                }
                return ['status' => 'success', 'msg' => '同步成功'];
            }
        }
    }

    return ['status' => 'error', 'msg' => '同步失败'];
}

// 删除API Key
function mailflow_TerminateAccount($params)
{
    mailflow_debug('开始删除API Key', ['username' => $params['username']]);

    $apiKeyID = mailflow_GetAPIKeyID($params);
    if (!$apiKeyID) {
        return ['status' => 'error', 'msg' => 'API Key不存在'];
    }

    $res = mailflow_Curl($params, "/admin/api/keys/{$apiKeyID}", [], 'DELETE', true);

    if (isset($res['message']) || (is_array($res) && empty($res['error']))) {
        return ['status' => 'success', 'msg' => 'API Key删除成功'];
    } else {
        return ['status' => 'error', 'msg' => $res['error'] ?? 'API Key删除失败'];
    }
}

// 暂停API Key
function mailflow_SuspendAccount($params)
{
    mailflow_debug('开始暂停API Key', ['username' => $params['username']]);

    $apiKeyID = mailflow_GetAPIKeyID($params);
    if (!$apiKeyID) {
        return ['status' => 'error', 'msg' => 'API Key不存在'];
    }

    $data = ['status' => 'inactive'];
    $res = mailflow_Curl($params, "/admin/api/keys/{$apiKeyID}", $data, 'PUT', true);

    if (isset($res['id']) || (is_array($res) && empty($res['error']))) {
        return ['status' => 'success', 'msg' => 'API Key已暂停'];
    } else {
        return ['status' => 'error', 'msg' => $res['error'] ?? 'API Key暂停失败'];
    }
}

// 恢复API Key
function mailflow_UnsuspendAccount($params)
{
    mailflow_debug('开始恢复API Key', ['username' => $params['username']]);

    $apiKeyID = mailflow_GetAPIKeyID($params);
    if (!$apiKeyID) {
        return ['status' => 'error', 'msg' => 'API Key不存在'];
    }

    $data = ['status' => 'active'];
    $res = mailflow_Curl($params, "/admin/api/keys/{$apiKeyID}", $data, 'PUT', true);

    if (isset($res['id']) || (is_array($res) && empty($res['error']))) {
        return ['status' => 'success', 'msg' => 'API Key已恢复'];
    } else {
        return ['status' => 'error', 'msg' => $res['error'] ?? 'API Key恢复失败'];
    }
}

// 通用API请求函数
function mailflow_Curl($params, $endpoint, $data = [], $method = 'GET', $useAdminToken = false, $apiKey = null)
{
    $protocol = 'http';
    $url = $protocol . '://' . $params['server_ip'] . ':' . $params['port'] . $endpoint;

    mailflow_debug('发送请求', [
        'url' => $url,
        'method' => $method,
        'useAdminToken' => $useAdminToken,
    ]);

    $curl = curl_init();

    $headers = ['Content-Type: application/json'];
    
    if ($useAdminToken) {
        $headers[] = 'X-Admin-Token: ' . $params['accesshash'];
    } elseif ($apiKey) {
        $headers[] = 'X-API-Key: ' . $apiKey;
    }

    $curlOptions = [
        CURLOPT_URL            => $url,
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_ENCODING       => '',
        CURLOPT_MAXREDIRS      => 10,
        CURLOPT_TIMEOUT        => 30,
        CURLOPT_CONNECTTIMEOUT => 10,
        CURLOPT_FOLLOWLOCATION => true,
        CURLOPT_HTTP_VERSION   => CURL_HTTP_VERSION_1_1,
        CURLOPT_CUSTOMREQUEST  => $method,
        CURLOPT_HTTPHEADER     => $headers,
    ];

    if ($method === 'POST' || $method === 'PUT') {
        $curlOptions[CURLOPT_POSTFIELDS] = json_encode($data);
    }

    curl_setopt_array($curl, $curlOptions);

    $response = curl_exec($curl);
    $errno    = curl_errno($curl);
    $httpCode = curl_getinfo($curl, CURLINFO_HTTP_CODE);
    $curlError = curl_error($curl);

    curl_close($curl);

    mailflow_debug('请求响应', [
        'http_code' => $httpCode,
        'response_length' => strlen($response),
        'curl_errno' => $errno,
    ]);

    if ($errno) {
        mailflow_debug('CURL错误', [
            'errno' => $errno,
            'error' => $curlError,
        ]);
        return null;
    }

    $decoded = json_decode($response, true);
    return $decoded;
}

// 获取API Key的数据库ID
function mailflow_GetAPIKeyID($params)
{
    if (empty($params['username'])) {
        return null;
    }

    $apiKey = $params['username'];
    $res = mailflow_Curl($params, '/admin/api/keys', [], 'GET', true);

    if (is_array($res)) {
        foreach ($res as $keyInfo) {
            if ($keyInfo['key'] === $apiKey) {
                return $keyInfo['id'];
            }
        }
    }

    return null;
}

