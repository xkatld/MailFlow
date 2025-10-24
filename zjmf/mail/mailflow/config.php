<?php
/*
 * MailFlow 邮件发送插件配置
 */
return [
    'api_url' => [
        'title' => 'Mailflow API地址',
        'type'  => 'text',
        'value' => 'http://localhost:8080',
        'tip'   => '例如: http://your-mailflow-server.com:8080',
    ],
    'api_key' => [
        'title' => 'API Key',
        'type'  => 'text',
        'value' => '',
        'tip'   => '在Mailflow管理后台创建的API Key',
    ],
    'timeout' => [
        'title' => '请求超时时间(秒)',
        'type'  => 'text',
        'value' => '30',
        'tip'   => '发送邮件的超时时间，默认30秒',
    ],
];

